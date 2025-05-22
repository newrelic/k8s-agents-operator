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
	"encoding/json"
	"errors"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/version"
)

const LicenseKey = "new_relic_license_key"

const (
	volumeName          = "newrelic-instrumentation"
	initContainerName   = "newrelic-instrumentation"
	apmConfigVolumeName = "newrelic-apm-config"
	apmConfigMountPath  = "/newrelic-apm-config"
)

const (
	EnvNewRelicAppName                   = "NEW_RELIC_APP_NAME"
	EnvNewRelicK8sOperatorEnabled        = "NEW_RELIC_K8S_OPERATOR_ENABLED"
	EnvNewRelicLabels                    = "NEW_RELIC_LABELS"
	EnvNewRelicLicenseKey                = "NEW_RELIC_LICENSE_KEY"
	DescK8sAgentOperatorVersionLabelName = "newrelic-k8s-agents-operator-version"
)

const instrumentationVersionAnnotation = "newrelic.com/instrumentation-versions"

var ErrInjectorAlreadyRegistered = errors.New("injector already registered in registry")

type Injector interface {
	// Deprecated: use InjectContainer from the ContainerInjector interface
	Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error)
	Language() string
	Accepts(inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) bool
	ConfigureClient(client client.Client)
	ConfigureLogger(logger logr.Logger)
}

// ContainerInjector is used to inject a specific container, rather than always the first container
type ContainerInjector interface {
	InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error)
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
	sort.Slice(injectors, func(i, j int) bool {
		return strings.Compare(injectors[i].Language(), injectors[j].Language()) < 0
	})
	return injectors
}

var DefaultInjectorRegistry = NewInjectorRegistry()

func getContainerIndex(pod corev1.Pod, containerName string) int {
	for i, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return i
		}
	}
	return -1
}

func getInitContainerIndex(pod corev1.Pod, initContainerName string) int {
	for i, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == initContainerName {
			return i
		}
	}
	return -1
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

func getValueFromEnv(envVars []corev1.EnvVar, name string) (string, bool) {
	for _, env := range envVars {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

// setEnvVar function sets env var to the container if not exist already.
// value of concatValues should be set to true if the env var supports multiple values separated by a string.  It will only be appended if it does not exist
// If it is set to false, the original container's env var value has priority.
func setEnvVar(container *corev1.Container, envVarName string, envVarValue string, concatValues bool, separator string) {
	idx := getIndexOfEnv(container.Env, envVarName)
	if idx < 0 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envVarName,
			Value: envVarValue,
		})
		return
	}
	if concatValues {
		if !strings.Contains(separator+container.Env[idx].Value+separator, separator+envVarValue+separator) {
			container.Env[idx].Value = container.Env[idx].Value + separator + envVarValue
		}
	}
}

func setContainerEnvFromInst(container *corev1.Container, inst current.Instrumentation) {
	for _, env := range inst.Spec.Agent.Env {
		if idx := getIndexOfEnv(container.Env, env.Name); idx == -1 {
			container.Env = append(container.Env, env)
		}
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
	lang   string
}

func (i *baseInjector) ConfigureLogger(logger logr.Logger) {
	i.logger = logger
}

func (i *baseInjector) ConfigureClient(client client.Client) {
	i.client = client
}

func (i *baseInjector) Accepts(inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.lang {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	if inst.Spec.LicenseKeySecret == "" {
		return false
	}
	return true
}

func (i *baseInjector) Language() string {
	return i.lang
}

// Deprecated: use setContainerEnvLicenseKey
func (i *baseInjector) injectNewrelicLicenseKeyIntoContainer(container corev1.Container, licenseKeySecretName string) corev1.Container {
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
	return container
}

// Deprecated: use setContainerEnvDefaults
func (i *baseInjector) injectNewrelicConfig(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, index int, licenseKeySecret string) corev1.Pod {
	pod = i.injectNewrelicEnvConfig(ctx, pod, index)
	pod.Spec.Containers[index] = i.injectNewrelicLicenseKeyIntoContainer(pod.Spec.Containers[index], licenseKeySecret)
	return pod
}

// Deprecated: use setContainerEnvInjectionDefaults;
func (i *baseInjector) injectNewrelicEnvConfig(ctx context.Context, pod corev1.Pod, index int) corev1.Pod {
	container := &pod.Spec.Containers[index]
	if idx := getIndexOfEnv(container.Env, EnvNewRelicAppName); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicAppName,
			Value: chooseServiceName(pod, index),
		})
	}
	if idx := getIndexOfEnv(container.Env, EnvNewRelicLabels); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicLabels,
			Value: "operator:auto-injection",
		})
	} else {
		labelAttributes := decodeAttributes(container.Env[idx].Value, ";", ":")
		labelAttributes["operator"] = "auto-injection"
		container.Env[idx].Value = encodeAttributes(labelAttributes, ";", ":")
	}
	if idx := getIndexOfEnv(container.Env, EnvNewRelicK8sOperatorEnabled); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicK8sOperatorEnabled,
			Value: "true",
		})
	}
	// Also apply specific pod labels indicating that operator is being attached and it's version
	applyLabelToPod(&pod, DescK8sAgentOperatorVersionLabelName, version.Get().Operator)
	return pod
}

func decodeAttributes(str string, fieldSeparator string, valueSeparator string) map[string]string {
	labelAttributes := map[string]string{}
	for _, attr := range strings.Split(str, fieldSeparator) {
		attrParts := strings.SplitN(attr, valueSeparator, 2)
		if len(attrParts) != 2 {
			continue
		}
		attrKey, attrValue := attrParts[0], attrParts[1]
		labelAttributes[attrKey] = attrValue
	}
	return labelAttributes
}

func encodeAttributes(m map[string]string, fieldSeparator string, valueSeparator string) string {
	str := ""
	i := 0
	keys := make([]string, len(m))
	for key := range m {
		keys[i] = key
		i++
	}
	slices.Sort(keys)
	for i, key := range keys {
		if i > 0 {
			str += fieldSeparator
		}
		str += key + valueSeparator + m[key]
	}
	return str
}

// Deprecated: use getAppName
func chooseServiceName(pod corev1.Pod, index int) string {
	for _, owner := range pod.ObjectMeta.OwnerReferences {
		switch strings.ToLower(owner.Kind) {
		case "deployment", "statefulset", "job", "cronjob":
			return owner.Name
		}
	}
	if pod.Name != "" {
		return pod.Name
	}
	return pod.Spec.Containers[index].Name
}

// Deprecated: use util.SetPodLabel
func applyLabelToPod(pod *corev1.Pod, key, val string) *corev1.Pod {
	labels := pod.Labels
	if labels == nil {
		pod.ObjectMeta.Labels = make(map[string]string)
	}
	pod.ObjectMeta.Labels[key] = val
	return pod
}

// Deprecated: use setPodAnnotationFromInstrumentationVersion
func addAnnotationToPodFromInstrumentationVersion(ctx context.Context, pod corev1.Pod, inst current.Instrumentation) corev1.Pod {
	logger := log.FromContext(ctx)
	instName := types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}.String()
	instVersions := map[string]string{}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if v, ok := pod.Annotations[instrumentationVersionAnnotation]; ok {
		err := json.Unmarshal([]byte(v), &instVersions)
		if err != nil {
			// crVersions could have incomplete data, however, some of the annotations may still be valid, so we'll keep them
			logger.Error(err, "Failed to unmarshal instrumentation version annotation, skipping adding new instrumentation version to pod annotation")
		}
	}
	instVersions[instName] = fmt.Sprintf("%s/%d", inst.UID, inst.Generation)
	instVersionBytes, err := json.Marshal(instVersions)
	if err != nil {
		logger.Error(err, "Failed to marshal instrumentation version annotation")
		return pod
	}
	pod.Annotations[instrumentationVersionAnnotation] = string(instVersionBytes)
	return pod
}

func setPodAnnotationFromInstrumentationVersion(pod *corev1.Pod, inst current.Instrumentation) error {
	instName := types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}.String()
	instVersions := map[string]string{}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if v, ok := pod.Annotations[instrumentationVersionAnnotation]; ok {
		if err := json.Unmarshal([]byte(v), &instVersions); err != nil {
			return fmt.Errorf("failed to unmarshal instrumentation version annotation, skipping adding new instrumentation version to pod annotation > %w", err)
		}
	}
	instVersions[instName] = fmt.Sprintf("%s/%d", inst.UID, inst.Generation)
	instVersionBytes, err := json.Marshal(instVersions)
	if err != nil {
		return fmt.Errorf("failed to marshal instrumentation version annotation > %w", err)
	}
	pod.Annotations[instrumentationVersionAnnotation] = string(instVersionBytes)
	return nil
}

func setAgentConfigMap(pod *corev1.Pod, configMapName string, container *corev1.Container) {
	if isPodVolumeMissing(*pod, apmConfigVolumeName) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: apmConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		})
	}

	if isContainerVolumeMissing(container, apmConfigVolumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      apmConfigVolumeName,
			MountPath: apmConfigMountPath,
		})
	}
}

func setContainerEnvLicenseKey(container *corev1.Container, licenseKeySecretName string) {
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

func setContainerEnvInjectionDefaults(container *corev1.Container) {
	if idx := getIndexOfEnv(container.Env, EnvNewRelicLabels); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicLabels,
			Value: "operator:auto-injection",
		})
	} else {
		labelAttributes := decodeAttributes(container.Env[idx].Value, ";", ":")
		labelAttributes["operator"] = "auto-injection"
		container.Env[idx].Value = encodeAttributes(labelAttributes, ";", ":")
	}
	if idx := getIndexOfEnv(container.Env, EnvNewRelicK8sOperatorEnabled); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicK8sOperatorEnabled,
			Value: "true",
		})
	}
}

func (i *baseInjector) setContainerEnvAppName(ctx context.Context, ns *corev1.Namespace, pod *corev1.Pod, container *corev1.Container) error {
	if idx := getIndexOfEnv(container.Env, EnvNewRelicAppName); idx == -1 {
		name, err := i.getRootResourceName(ctx, ns, pod)
		if err != nil {
			return fmt.Errorf("failed to get root resource name for pod > %w", err)
		}
		if name == "" {
			name = container.Name
		}
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicAppName,
			Value: name,
		})
	}
	return nil
}

func (i *baseInjector) getRootResourceName(ctx context.Context, ns *corev1.Namespace, pod *corev1.Pod) (string, error) {
	for _, owner := range pod.ObjectMeta.OwnerReferences {
		switch strings.ToLower(owner.Kind) {
		case "deployment", "statefulset", "cronjob", "daemonset":
			return owner.Name, nil
		}
	}
	checkError := func(err error) bool { return apierrors.IsNotFound(err) }
	backOff := wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.5, Jitter: 0.1, Steps: 20, Cap: 2 * time.Second}
	for _, owner := range pod.ObjectMeta.OwnerReferences {
		ownerName := types.NamespacedName{Namespace: ns.Name, Name: owner.Name}
		switch strings.ToLower(owner.Kind) {
		case "job":
			obj := batchv1.Job{}
			if err := retry.OnError(backOff, checkError, func() error { return i.client.Get(ctx, ownerName, &obj) }); err != nil {
				return "", fmt.Errorf("failed to get parent name > %w", err)
			}
			for _, parentOwner := range obj.ObjectMeta.OwnerReferences {
				if strings.ToLower(parentOwner.Kind) == "cronjob" {
					return parentOwner.Name, nil
				}
			}
			return owner.Name, nil
		case "replicaset":
			obj := appsv1.ReplicaSet{}
			if err := retry.OnError(backOff, checkError, func() error { return i.client.Get(ctx, ownerName, &obj) }); err != nil {
				return "", fmt.Errorf("failed to get parent name > %w", err)
			}
			for _, parentOwner := range obj.ObjectMeta.OwnerReferences {
				if strings.ToLower(parentOwner.Kind) == "deployment" {
					return parentOwner.Name, nil
				}
			}
			return owner.Name, nil
		}
	}
	return pod.Name, nil
}

func insertContainerBeforeIndex(containers []corev1.Container, index int, newContainer corev1.Container) []corev1.Container {
	initContainers := make([]corev1.Container, len(containers)+1)
	copy(initContainers, containers[0:index])
	initContainers[index] = newContainer
	copy(initContainers[index+1:], containers[index:])
	return initContainers
}

func removeContainerByIndex(containers []corev1.Container, index int) ([]corev1.Container, corev1.Container) {
	oldContainer := containers[index]
	newContainers := make([]corev1.Container, len(containers)-1)
	copy(newContainers, containers[0:index])
	copy(newContainers[index:], containers[index+1:])
	return newContainers, oldContainer
}
