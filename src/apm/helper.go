/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package apm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.5.0"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
)

const LicenseKey = "new_relic_license_key"

const (
	volumeName        = "newrelic-instrumentation"
	initContainerName = "newrelic-instrumentation"
)

const (
	EnvNewRelicAppName            = "NEW_RELIC_APP_NAME"
	EnvNewRelicK8sOperatorEnabled = "NEW_RELIC_K8S_OPERATOR_ENABLED"
	EnvNewRelicLabels             = "NEW_RELIC_LABELS"
	EnvNewRelicLicenseKey         = "NEW_RELIC_LICENSE_KEY"
)

var ErrInjectorAlreadyRegistered = errors.New("injector already registered in registry")

type Injector interface {
	Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error)
	Language() string
	ConfigureClient(client client.Client)
	ConfigureLogger(logger logr.Logger)
}

type Injectors []Injector

func (i Injectors) Names() []string {
	injectorNames := make([]string, len(i))
	for j, injector := range i {
		injectorNames[j] = injector.Language()
	}
	return injectorNames
}

type InjectorRegistery struct {
	injectors   []Injector
	injectorMap map[string]struct{}
	mu          *sync.Mutex
}

func NewInjectorRegistry() *InjectorRegistery {
	return &InjectorRegistery{
		injectorMap: make(map[string]struct{}),
		mu:          &sync.Mutex{},
	}
}

func (ir *InjectorRegistery) Register(injector Injector) error {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	if _, ok := ir.injectorMap[injector.Language()]; ok {
		return ErrInjectorAlreadyRegistered
	}
	ir.injectors = append(ir.injectors, injector)
	return nil
}

func (ir *InjectorRegistery) MustRegister(injector Injector) {
	err := ir.Register(injector)
	if err != nil {
		panic(err)
	}
}

func (ir *InjectorRegistery) GetInjectors() Injectors {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	injectors := make([]Injector, len(ir.injectors))
	copy(injectors, ir.injectors)
	return injectors
}

var DefaultInjectorRegistry = NewInjectorRegistry()

func getContainerIndex(containerName string, pod corev1.Pod) int {
	// We search for specific container to inject variables and if no one is found
	// We fallback to first container
	var index = 0
	for idx, ctnair := range pod.Spec.Containers {
		if ctnair.Name == containerName {
			index = idx
		}
	}

	return index
}

// Calculate if we already inject InitContainers.
func isInitContainerMissing(pod corev1.Pod, initContainerName string) bool {
	for _, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == initContainerName {
			return false
		}
	}
	return true
}

// Calculate if we already inject a Volume.
func isPodVolumeMissing(pod corev1.Pod, volumeName string) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == volumeName {
			return false
		}
	}
	return true
}

// Calculate if we already inject a Volume.
func isContainerVolumeMissing(container *corev1.Container, volumeName string) bool {
	for _, volume := range container.VolumeMounts {
		if volume.Name == volumeName {
			return false
		}
	}
	return true
}

func getIndexOfEnv(envs []corev1.EnvVar, name string) int {
	for i := range envs {
		if envs[i].Name == name {
			return i
		}
	}
	return -1
}

// setEnvVar function sets env var to the container if not exist already.
// value of concatValues should be set to true if the env var supports multiple values separated by :.
// If it is set to false, the original container's env var value has priority.
func setEnvVar(container *corev1.Container, envVarName string, envVarValue string, concatValues bool) {
	idx := getIndexOfEnv(container.Env, envVarName)
	if idx < 0 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envVarName,
			Value: envVarValue,
		})
		return
	}
	if concatValues {
		container.Env[idx].Value = fmt.Sprintf("%s:%s", container.Env[idx].Value, envVarValue)
	}
}

func validateContainerEnv(envs []corev1.EnvVar, envsToBeValidated ...string) error {
	for _, envToBeValidated := range envsToBeValidated {
		for _, containerEnv := range envs {
			if containerEnv.Name == envToBeValidated {
				if containerEnv.ValueFrom != nil {
					return fmt.Errorf("the container defines env var value via ValueFrom, envVar: %s", containerEnv.Name)
				}
				break
			}
		}
	}
	return nil
}

type baseInjector struct {
	logger logr.Logger
	client client.Client
}

func (i *baseInjector) ConfigureLogger(logger logr.Logger) {
	i.logger = logger
}

func (i *baseInjector) ConfigureClient(client client.Client) {
	i.client = client
}

func (i *baseInjector) validate(inst v1alpha2.Instrumentation) error {
	if inst.Spec.LicenseKeySecret == "" {
		return fmt.Errorf("licenseKeySecret must not be blank")
	}
	return nil
}

func (i *baseInjector) injectNewrelicLicenseKeyIntoContainer(container *corev1.Container, licenseKeySecretName string) {
	if idx := getIndexOfEnv(container.Env, EnvNewRelicLicenseKey); idx == -1 {
		optional := true
		container.Env = append(container.Env, corev1.EnvVar{
			Name: EnvNewRelicLicenseKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: licenseKeySecretName},
					Key:                  LicenseKey,
					Optional:             &optional,
				},
			},
		})
	}
}

func (i *baseInjector) injectNewrelicConfig(ctx context.Context, resource v1alpha2.Resource, ns corev1.Namespace, pod corev1.Pod, index int, licenseKeySecret string) corev1.Pod {
	i.injectNewrelicEnvConfig(ctx, resource, ns, pod, index)
	i.injectNewrelicLicenseKeyIntoContainer(&pod.Spec.Containers[index], licenseKeySecret)
	return pod
}

func (i *baseInjector) injectNewrelicEnvConfig(ctx context.Context, resource v1alpha2.Resource, ns corev1.Namespace, pod corev1.Pod, index int) corev1.Pod {
	container := &pod.Spec.Containers[index]
	if idx := getIndexOfEnv(container.Env, EnvNewRelicAppName); idx == -1 {
		//@todo: how can we do this if multiple injectors need this?
		resourceMap := i.createResourceMap(ctx, resource, ns, pod, index)
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicAppName,
			Value: chooseServiceName(pod, resourceMap, index),
		})
	}
	if idx := getIndexOfEnv(container.Env, EnvNewRelicLabels); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicLabels,
			Value: "operator:auto-injection",
		})
	}
	if idx := getIndexOfEnv(container.Env, EnvNewRelicK8sOperatorEnabled); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicK8sOperatorEnabled,
			Value: "true",
		})
	}
	return pod
}

// createResourceMap creates resource attribute map.
// User defined attributes (in explicitly set env var) have higher precedence.
func (i *baseInjector) createResourceMap(ctx context.Context, resource v1alpha2.Resource, ns corev1.Namespace, pod corev1.Pod, index int) map[string]string {

	//@todo: revise this? this is specific to the golang apm
	// get existing resources env var and parse it into a map
	existingRes := map[string]bool{}
	existingResourceEnvIdx := getIndexOfEnv(pod.Spec.Containers[index].Env, EnvOTELResourceAttrs)
	if existingResourceEnvIdx > -1 {
		existingResArr := strings.Split(pod.Spec.Containers[index].Env[existingResourceEnvIdx].Value, ",")
		for _, kv := range existingResArr {
			keyValueArr := strings.Split(strings.TrimSpace(kv), "=")
			if len(keyValueArr) != 2 {
				continue
			}
			existingRes[keyValueArr[0]] = true
		}
	}

	res := map[string]string{}
	for k, v := range resource.Attributes {
		if !existingRes[k] {
			res[k] = v
		}
	}
	k8sResources := map[attribute.Key]string{}
	k8sResources[semconv.K8SNamespaceNameKey] = ns.Name
	k8sResources[semconv.K8SContainerNameKey] = pod.Spec.Containers[index].Name
	// Some fields might be empty - node name, pod name
	// The pod name might be empty if the pod is created form deployment template
	k8sResources[semconv.K8SPodNameKey] = pod.Name
	k8sResources[semconv.K8SPodUIDKey] = string(pod.UID)
	k8sResources[semconv.K8SNodeNameKey] = pod.Spec.NodeName
	k8sResources[semconv.ServiceInstanceIDKey] = createServiceInstanceId(ns.Name, pod.Name, pod.Spec.Containers[index].Name)
	i.addParentResourceLabels(ctx, resource.AddK8sUIDAttributes, ns, pod.ObjectMeta, k8sResources)
	for k, v := range k8sResources {
		if !existingRes[string(k)] && v != "" {
			res[string(k)] = v
		}
	}
	return res
}

func (i *baseInjector) addParentResourceLabels(ctx context.Context, uid bool, ns corev1.Namespace, objectMeta metav1.ObjectMeta, resources map[attribute.Key]string) {
	for _, owner := range objectMeta.OwnerReferences {
		switch strings.ToLower(owner.Kind) {
		case "replicaset":
			resources[semconv.K8SReplicaSetNameKey] = owner.Name
			if uid {
				resources[semconv.K8SReplicaSetUIDKey] = string(owner.UID)
			}
			// parent of ReplicaSet is e.g. Deployment which we are interested to know
			rs := appsv1.ReplicaSet{}
			nsn := types.NamespacedName{Namespace: ns.Name, Name: owner.Name}
			backOff := wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.5, Jitter: 0.1, Steps: 20, Cap: 2 * time.Second}

			checkError := func(err error) bool {
				return apierrors.IsNotFound(err)
			}

			getReplicaSet := func() error {
				return i.client.Get(ctx, nsn, &rs)
			}

			// use a retry loop to get the Deployment. A single call to client.get fails occasionally
			err := retry.OnError(backOff, checkError, getReplicaSet)
			if err != nil {
				i.logger.Error(err, "failed to get replicaset", "replicaset", nsn.Name, "namespace", nsn.Namespace)
			}
			i.addParentResourceLabels(ctx, uid, ns, rs.ObjectMeta, resources)
		case "deployment":
			resources[semconv.K8SDeploymentNameKey] = owner.Name
			if uid {
				resources[semconv.K8SDeploymentUIDKey] = string(owner.UID)
			}
		case "statefulset":
			resources[semconv.K8SStatefulSetNameKey] = owner.Name
			if uid {
				resources[semconv.K8SStatefulSetUIDKey] = string(owner.UID)
			}
		case "daemonset":
			resources[semconv.K8SDaemonSetNameKey] = owner.Name
			if uid {
				resources[semconv.K8SDaemonSetUIDKey] = string(owner.UID)
			}
		case "job":
			resources[semconv.K8SJobNameKey] = owner.Name
			if uid {
				resources[semconv.K8SJobUIDKey] = string(owner.UID)
			}
		case "cronjob":
			resources[semconv.K8SCronJobNameKey] = owner.Name
			if uid {
				resources[semconv.K8SCronJobUIDKey] = string(owner.UID)
			}
		}
	}
}

func chooseServiceName(pod corev1.Pod, resources map[string]string, index int) string {
	if name := resources[string(semconv.K8SDeploymentNameKey)]; name != "" {
		return name
	}
	if name := resources[string(semconv.K8SStatefulSetNameKey)]; name != "" {
		return name
	}
	if name := resources[string(semconv.K8SJobNameKey)]; name != "" {
		return name
	}
	if name := resources[string(semconv.K8SCronJobNameKey)]; name != "" {
		return name
	}
	if name := resources[string(semconv.K8SPodNameKey)]; name != "" {
		return name
	}
	return pod.Spec.Containers[index].Name
}

// creates the service.instance.id following the semantic defined by
// https://github.com/open-telemetry/semantic-conventions/pull/312.
func createServiceInstanceId(namespaceName, podName, containerName string) string {
	var serviceInstanceId string
	if namespaceName != "" && podName != "" && containerName != "" {
		resNames := []string{namespaceName, podName, containerName}
		serviceInstanceId = strings.Join(resNames, ".")
	}
	return serviceInstanceId
}
