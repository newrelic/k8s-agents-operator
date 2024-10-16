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
package instrumentation

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
	"github.com/newrelic/k8s-agents-operator/src/internal/webhookhandler"
)

// compile time type assertion
var (
	_ webhookhandler.PodMutator = (*instPodMutator)(nil)
	_ InstrumentationLocator    = (*NewrelicInstrumentationLocator)(nil)
	_ SdkInjector               = (*NewrelicSdkInjector)(nil)
	_ SecretReplicator          = (*NewrelicSecretReplicator)(nil)
	_ ConfigMapReplicator       = (*NewrelicConfigMapReplicator)(nil)
)

var (
	errMultipleInstancesPossible = errors.New("multiple New Relic Instrumentation instances available, cannot determine which one to select")
	errNoInstancesAvailable      = errors.New("no New Relic Instrumentation instances available")
)

type instPodMutator struct {
	logger                 logr.Logger
	client                 client.Client
	sdkInjector            SdkInjector
	secretReplicator       SecretReplicator
	configMapReplicator    ConfigMapReplicator
	instrumentationLocator InstrumentationLocator
	operatorNamespace      string
}

// NewMutator is used to get a new instance of a mutator
func NewMutator(
	logger logr.Logger,
	client client.Client,
	sdkInjector SdkInjector,
	secretReplicator SecretReplicator,
	configMapReplicator ConfigMapReplicator,
	instrumentationLocator InstrumentationLocator,
	operatorNamespace string,
) *instPodMutator {
	return &instPodMutator{
		logger:                 logger,
		client:                 client,
		sdkInjector:            sdkInjector,
		secretReplicator:       secretReplicator,
		configMapReplicator:    configMapReplicator,
		instrumentationLocator: instrumentationLocator,
		operatorNamespace:      operatorNamespace,
	}
}

// Mutate is used to mutate a pod based on some instrumentation(s)
func (pm *instPodMutator) Mutate(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	logger := pm.logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

	instCandidates, err := pm.instrumentationLocator.GetInstrumentations(ctx, ns, pod)
	if err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return pod, err
	}
	if len(instCandidates) == 0 {
		logger.Info("no New Relic Instrumentation instance for this Pod")
		return pod, errNoInstancesAvailable
	}

	instrumentations, err := GetLanguageInstrumentations(instCandidates)
	if err != nil {
		if errors.Is(err, errMultipleInstancesPossible) {
			pm.logger.Info("too many New Relic Instrumentation instances for this Pod.  only 1 allowed")
		} else {
			logger.Error(err, "failed to select a New Relic Instrumentation instance for this Pod")
		}
		return pod, err
	}

	licenseKeySecret, err := GetSecretNameFromInstrumentations(instCandidates)
	if err != nil {
		logger.Error(err, "failed to identify the correct secret.  all matching instrumentation's must use the same secret")
		return pod, nil
	}

	if err = pm.secretReplicator.ReplicateSecret(ctx, ns, pod, pm.operatorNamespace, licenseKeySecret); err != nil {
		logger.Error(err, "failed to replicate secret")
		return pod, nil
	}

	if err := ValidateAgentEnvInInstrumentations(instrumentations); err != nil {
		logger.Error(err, "failed to validate instrumentations")
		return pod, err
	}

	uniqueSecretNames := map[string]struct{}{}
	uniqueConfigMapNames := map[string]struct{}{}
	for _, instrumentation := range instrumentations {
		uniqueSecretList := CollectSecretsFromInstrumentation(*instrumentation)
		for _, secretName := range uniqueSecretList {
			uniqueSecretNames[secretName] = struct{}{}
		}
		uniqueConfigMapList := CollectConfigMapsFromInstrumentation(*instrumentation)
		for _, configMapName := range uniqueConfigMapList {
			uniqueConfigMapNames[configMapName] = struct{}{}
		}
	}
	uniqueSecretList := make([]string, 0, len(uniqueSecretNames))
	for secretName := range uniqueSecretNames {
		uniqueSecretList = append(uniqueSecretList, secretName)
	}
	uniqueConfigMapList := make([]string, 0, len(uniqueConfigMapNames))
	for configMapName := range uniqueConfigMapNames {
		uniqueConfigMapList = append(uniqueConfigMapList, configMapName)
	}
	//@todo: remove interface conversion once the tests support this
	if secretsReplicator, ok := pm.secretReplicator.(SecretsReplicator); ok {
		if err = secretsReplicator.ReplicateSecrets(ctx, ns, pod, pm.operatorNamespace, uniqueSecretList); err != nil {
			logger.Error(err, "failed to replicate secrets")
			return pod, err
		}
	}
	//@todo: remove interface conversion once the tests support this
	if configMapsReplicator, ok := pm.configMapReplicator.(ConfigMapsReplicator); ok {
		if err = configMapsReplicator.ReplicateConfigMaps(ctx, ns, pod, pm.operatorNamespace, uniqueConfigMapList); err != nil {
			logger.Error(err, "failed to replicate config maps")
			return pod, err
		}
	}

	return pm.sdkInjector.Inject(ctx, instrumentations, ns, pod), nil
}

// ValidateAgentEnvInInstrumentations is used to ensure that no duplicate env keys are duplicated
func ValidateAgentEnvInInstrumentations(instrumentations []*v1alpha2.Instrumentation) error {
	uniqueNames := map[string]int{}
	for _, instrumentation := range instrumentations {
		for _, entry := range instrumentation.Spec.Agent.Env {
			uniqueNames[entry.Name]++
		}
	}
	var errs []error
	for name, count := range uniqueNames {
		if count > 1 {
			errs = append(errs, fmt.Errorf("env variable %q is duplicated", name))
		}
	}
	return errors.Join(errs...)
}

// CollectConfigMapsFromInstrumentation to find all the unique configmap names from the agent.env
func CollectConfigMapsFromInstrumentation(instrumentation v1alpha2.Instrumentation) []string {
	uniqueNames := map[string]struct{}{}
	for _, entry := range instrumentation.Spec.Agent.Env {
		if entry.ValueFrom == nil {
			continue
		}
		if entry.ValueFrom.ConfigMapKeyRef == nil {
			continue
		}
		if entry.ValueFrom.ConfigMapKeyRef.Name == "" {
			continue
		}
		uniqueNames[entry.ValueFrom.ConfigMapKeyRef.Name] = struct{}{}
	}
	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}
	return names
}

// CollectConfigMapsFromInstrumentation to find all the unique secret names from the agent.env
func CollectSecretsFromInstrumentation(instrumentation v1alpha2.Instrumentation) []string {
	uniqueNames := map[string]struct{}{}
	for _, entry := range instrumentation.Spec.Agent.Env {
		if entry.ValueFrom == nil {
			continue
		}
		if entry.ValueFrom.SecretKeyRef == nil {
			continue
		}
		if entry.ValueFrom.SecretKeyRef.Name == "" {
			continue
		}
		uniqueNames[entry.ValueFrom.SecretKeyRef.Name] = struct{}{}
	}
	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}
	return names
}

// GetLanguageInstrumentations is used to collect all instrumentations and validate that only a single instrumentation
// exists for each language, and return them together, modifying the slice items in place
func GetLanguageInstrumentations(instCandidates []*v1alpha2.Instrumentation) ([]*v1alpha2.Instrumentation, error) {
	languages := map[string]*v1alpha2.Instrumentation{}
	i := 0
	for _, candidate := range instCandidates {
		if currentInst, ok := languages[candidate.Spec.Agent.Language]; ok {
			if !currentInst.Spec.Agent.IsEqual(candidate.Spec.Agent) {
				return nil, errMultipleInstancesPossible
			}
		} else {
			languages[candidate.Spec.Agent.Language] = candidate
			instCandidates[i] = candidate
			i++
		}
	}
	return instCandidates[:i], nil
}

// InstrumentationLocator is used to find instrumentations
type InstrumentationLocator interface {
	GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*v1alpha2.Instrumentation, error)
}

// NewrelicInstrumentationLocator is the base struct for locating instrumentations
type NewrelicInstrumentationLocator struct {
	logger            logr.Logger
	client            client.Client
	operatorNamespace string
}

// NewNewRelicInstrumentationLocator is the constructor for getting instrumentations
func NewNewRelicInstrumentationLocator(logger logr.Logger, client client.Client, operatorNamespace string) *NewrelicInstrumentationLocator {
	return &NewrelicInstrumentationLocator{
		logger:            logger,
		client:            client,
		operatorNamespace: operatorNamespace,
	}
}

// GetInstrumentations is used to get all instrumentations in the cluster. While we could limit it to the operator
// namespace, it's more helpful to list anything in the logs that may have been excluded.
func (il *NewrelicInstrumentationLocator) GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*v1alpha2.Instrumentation, error) {
	logger := il.logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

	var listInst v1alpha2.InstrumentationList
	if err := il.client.List(ctx, &listInst); err != nil {
		return nil, err
	}

	//nolint:prealloc
	var candidates []*v1alpha2.Instrumentation
	for _, inst := range listInst.Items {
		if inst.Namespace != il.operatorNamespace {
			logger.Info("ignoring instrumentation not in operator namespace",
				"instrumentation_name", inst.Name,
				"instrumentation_namespace", inst.Namespace,
				"operator_namespace", il.operatorNamespace,
			)
			continue
		}
		podSelector, err := metav1.LabelSelectorAsSelector(&inst.Spec.PodLabelSelector)
		if err != nil {
			logger.Error(err, "failed to parse pod label selector",
				"instrumentation_name", inst.Name,
				"instrumentation_namespace", inst.Namespace,
			)
			continue
		}
		namespaceSelector, err := metav1.LabelSelectorAsSelector(&inst.Spec.NamespaceLabelSelector)
		if err != nil {
			logger.Error(err, "failed to parse namespace label selector",
				"instrumentation_name", inst.Name,
				"instrumentation_namespace", inst.Namespace,
			)
			continue
		}

		if !podSelector.Matches(fields.Set(pod.Labels)) {
			continue
		}
		if !namespaceSelector.Matches(fields.Set(ns.Labels)) {
			continue
		}

		logger.Info("matching instrumentation",
			"instrumentation_name", inst.Name,
			"instrumentation_namespace", inst.Namespace,
		)

		if inst.Spec.LicenseKeySecret == "" {
			inst.Spec.LicenseKeySecret = DefaultLicenseKeySecretName
		}
		candidates = append(candidates, &inst)
	}
	return candidates, nil
}

// GetSecretNameFromInstrumentations is used to get a single secret key name from a list of Instrumentation's.  It will
// use the default if none is provided.  If any of them are different by name, this will fail, as we can only bind a
// single license key to a single pod.
func GetSecretNameFromInstrumentations(insts []*v1alpha2.Instrumentation) (string, error) {
	secretName := ""
	for _, inst := range insts {
		specSecretName := inst.Spec.LicenseKeySecret
		if specSecretName == "" {
			specSecretName = DefaultLicenseKeySecretName
		}
		if secretName == "" {
			secretName = specSecretName
		}
		if secretName != specSecretName {
			return "", errors.New("multiple key secrets")
		}
	}
	return secretName, nil
}

// SecretReplicator is used to copy secrets from one namespace to another
type SecretReplicator interface {
	ReplicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error
}

// SecretReplicator is used to copy secrets from one namespace to another
type SecretsReplicator interface {
	ReplicateSecrets(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretNames []string) error
}

// ConfigMapReplicator is used to copy configmaps from one namespace to another
type ConfigMapReplicator interface {
	ReplicateConfigMap(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapName string) error
}

// ConfigMapReplicator is used to copy configmaps from one namespace to another
type ConfigMapsReplicator interface {
	ReplicateConfigMaps(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapNames []string) error
}

// NewrelicSecretReplicator is the base struct used for copying the secrets
type NewrelicSecretReplicator struct {
	client client.Client
	logger logr.Logger
}

// NewNewrelicSecretReplicator is the constructor for copying secrets
func NewNewrelicSecretReplicator(logger logr.Logger, client client.Client) *NewrelicSecretReplicator {
	return &NewrelicSecretReplicator{client: client, logger: logger}
}

func (sr *NewrelicSecretReplicator) ReplicateSecrets(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretNames []string) error {
	for _, secretName := range secretNames {
		if err := sr.ReplicateSecret(ctx, ns, pod, operatorNamespace, secretName); err != nil {
			return err
		}
	}
	return nil
}

// ReplicateSecret is used to copy the secret from the operator namespace to the pod namespace if the secret doesn't already exist
func (sr *NewrelicSecretReplicator) ReplicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error {
	logger := sr.logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

	var secret corev1.Secret

	if secretName == "" {
		secretName = DefaultLicenseKeySecretName
	}

	err := sr.client.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: secretName}, &secret)
	if err == nil {
		logger.Info("secret already exists")
		return nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "failed to check for existing secret in pod namespace")
		return err
	}
	logger.Info("replicating secret to pod namespace")

	if err = sr.client.Get(ctx, client.ObjectKey{Namespace: operatorNamespace, Name: secretName}, &secret); err != nil {
		logger.Error(err, "failed to retrieve the secret from operator namespace")
		return err
	}

	newSecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns.Name,
		},
		Data: secret.Data,
	}
	if err = sr.client.Create(ctx, &newSecret); err != nil {
		logger.Error(err, "failed to create a new secret")
		return err
	}

	return nil
}

type NewrelicConfigMapReplicator struct {
	client client.Client
	logger logr.Logger
}

// NewNewrelicConfigMapReplicator is the constructor for copying configmaps
func NewNewrelicConfigMapReplicator(logger logr.Logger, client client.Client) *NewrelicConfigMapReplicator {
	return &NewrelicConfigMapReplicator{client: client, logger: logger}
}

func (cr *NewrelicConfigMapReplicator) ReplicateConfigMaps(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapNames []string) error {
	for _, configMapName := range configMapNames {
		if err := cr.ReplicateConfigMap(ctx, ns, pod, operatorNamespace, configMapName); err != nil {
			return err
		}
	}
	return nil
}

func (cr *NewrelicConfigMapReplicator) ReplicateConfigMap(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapName string) error {
	logger := cr.logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

	var configMap corev1.ConfigMap

	if configMapName == "" {
		return fmt.Errorf("config map name required")
	}

	err := cr.client.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: configMapName}, &configMap)
	if err == nil {
		logger.Info("config map already exists")
		return nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "failed to check for existing config map in pod namespace")
		return err
	}
	logger.Info("replicating config map to pod namespace")

	if err = cr.client.Get(ctx, client.ObjectKey{Namespace: operatorNamespace, Name: configMapName}, &configMap); err != nil {
		logger.Error(err, "failed to retrieve the config map from operator namespace")
		return err
	}

	newConfigMap := corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: ns.Name,
		},
		Data: configMap.Data,
	}
	if err = cr.client.Create(ctx, &newConfigMap); err != nil {
		logger.Error(err, "failed to create a new config map")
		return err
	}

	return nil
}
