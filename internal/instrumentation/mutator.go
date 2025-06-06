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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/util"
)

const (
	WellKnownLabelManagedBy = "app.kubernetes.io/managed-by"
	ManagedBy               = "newrelic-k8s-agents-operator"
)

type (
	// SecretReplicator is used to copy a secret from one namespace to another
	SecretReplicator interface {
		ReplicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error
	}

	// SecretsReplicator is used to copy secrets from one namespace to another
	SecretsReplicator interface {
		ReplicateSecrets(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretNames []string) error
	}

	// ConfigMapReplicator is used to copy a configmap from one namespace to another
	ConfigMapReplicator interface {
		ReplicateConfigMap(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapName string) error
	}

	// ConfigMapsReplicator is used to copy configmapd from one namespace to another
	ConfigMapsReplicator interface {
		ReplicateConfigMaps(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapNames []string) error
	}

	// InstrumentationLocator is used to find instrumentations
	InstrumentationLocator interface {
		GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error)
	}
)

// compile time type assertion
var (
	_ InstrumentationLocator = (*NewrelicInstrumentationLocator)(nil)
	_ SdkContainerInjector   = (*NewrelicSdkInjector)(nil)
	_ SecretReplicator       = (*NewrelicSecretReplicator)(nil)
	_ SecretsReplicator      = (*NewrelicSecretReplicator)(nil)
	_ ConfigMapReplicator    = (*NewrelicConfigMapReplicator)(nil)
	_ ConfigMapsReplicator   = (*NewrelicConfigMapReplicator)(nil)
)

var (
	errMultipleInstancesPossible = errors.New("multiple New Relic Instrumentation instances available, cannot determine which one to select")
	errNoInstancesAvailable      = errors.New("no New Relic Instrumentation instances available")
	errMultipleSecretsPossible   = errors.New("multiple license key secrets possible")
)

type InstrumentationPodMutator struct {
	logger                 logr.Logger
	client                 client.Client
	sdkInjector            SdkContainerInjector
	secretReplicator       SecretsReplicator
	configMapReplicator    ConfigMapsReplicator
	instrumentationLocator InstrumentationLocator
	operatorNamespace      string

	envSecretReplicationIsEnabled      bool
	envConfigMapReplicationIsEnabled   bool
	agentConfigMapReplicationIsEnabled bool
	secretReplicationIsEnabled         bool
	configMapReplicationIsEnabled      bool
}

// NewMutator is used to get a new instance of a mutator
func NewMutator(
	logger logr.Logger,
	client client.Client,
	sdkInjector SdkContainerInjector,
	secretReplicator SecretsReplicator,
	configMapReplicator ConfigMapsReplicator,
	instrumentationLocator InstrumentationLocator,
	operatorNamespace string,
) *InstrumentationPodMutator {
	return &InstrumentationPodMutator{
		logger:                     logger,
		client:                     client,
		sdkInjector:                sdkInjector,
		secretReplicator:           secretReplicator,
		configMapReplicator:        configMapReplicator,
		instrumentationLocator:     instrumentationLocator,
		operatorNamespace:          operatorNamespace,
		secretReplicationIsEnabled: true,
	}
}

// Mutate is used to mutate a pod based on some instrumentation(s)
func (pm *InstrumentationPodMutator) Mutate(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	logger := pm.logger.WithValues("namespace", pod.Namespace, "name", pod.Name, "generate_name", pod.GenerateName)

	instCandidates, err := pm.instrumentationLocator.GetInstrumentations(ctx, ns, pod)
	if err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return corev1.Pod{}, err
	}
	if len(instCandidates) == 0 {
		logger.Info("no New Relic Instrumentation instance for this Pod")
		return corev1.Pod{}, errNoInstancesAvailable
	}
	logger.Info("New Relic Instrumentation instance for this Pod", "count", len(instCandidates))

	// filter by container name; map[container.Name]=[instrumentation, instrtumentations...]
	instrumentations := filterInstrumentations(instCandidates, &pod)

	// sort, so that, given the same list of instrumentations, we always apply the mutations in the same predictable order
	instrumentations = sortInstrumentations(instrumentations)

	// remove duplicate instrumentations by language (if identical, otherwise error)
	instrumentations, err = validateAndReduceInstrumentations(instrumentations)
	if err != nil {
		if errors.Is(err, errMultipleInstancesPossible) {
			pm.logger.Info("too many Instrumentation instances for the Pod Container")
		} else {
			logger.Error(err, "failed to select a New Relic Instrumentation instance for this Pod")
		}
		return corev1.Pod{}, err
	}

	// ensure we have only one license key per container
	if err := validateInstrumentationLicenseKeySecrets(instrumentations); err != nil {
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this Pod")
		return corev1.Pod{}, err
	}

	// ensure we only have 1 health agent
	if err := validateHealthAgents(instrumentations); err != nil {
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this Pod")
		return corev1.Pod{}, err
	}

	// ensure that the health sidecar is not being used with an init container
	if err := validateContainerWithHealthAgent(instrumentations, &pod); err != nil {
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this Pod")
		return corev1.Pod{}, err
	}

	if pm.secretReplicationIsEnabled {
		secrets := GetLicenseKeySecretsFromInstrumentations(instrumentations)
		if pm.envSecretReplicationIsEnabled {
			envSecrets := GetSecretsFromInstrumentationsAgentEnv(instrumentations)
			secrets = append(secrets, envSecrets...)
		}
		secrets = GetUniqStrings(secrets)
		if err = pm.secretReplicator.ReplicateSecrets(ctx, ns, pod, pm.operatorNamespace, secrets); err != nil {
			logger.Error(err, "failed to replicate secrets")
		}
	}

	if pm.configMapReplicationIsEnabled {
		var configMaps []string
		if pm.agentConfigMapReplicationIsEnabled {
			configMaps = GetAgentConfigMapsFromInstrumentations(instrumentations)
		}
		if pm.envConfigMapReplicationIsEnabled {
			envConfigMaps := GetConfigMapsFromInstrumentationsAgentEnv(instrumentations)
			configMaps = append(configMaps, envConfigMaps...)
		}
		configMaps = GetUniqStrings(configMaps)
		if err = pm.configMapReplicator.ReplicateConfigMaps(ctx, ns, pod, pm.operatorNamespace, configMaps); err != nil {
			logger.Error(err, "failed to replicate config maps")
		}
	}

	return pm.sdkInjector.InjectContainers(ctx, instrumentations, ns, pod), nil
}

// reduceIdenticalInstrumentations is used to collect all instrumentations and so that only a single instrumentation
// exists for each language, removing duplicates, and return them together, modifying the slice items in place
func reduceIdenticalInstrumentations(instCandidates []*current.Instrumentation) []*current.Instrumentation {
	languages := map[string]*current.Instrumentation{}
	i := 0
	for _, candidate := range instCandidates {
		if _, ok := languages[candidate.Spec.Agent.Language]; !ok {
			languages[candidate.Spec.Agent.Language] = candidate
			instCandidates[i] = candidate
			i++
		}
	}
	return instCandidates[:i]
}

// validateInstrumentations is used to validate that only a single instrumentation exists for each language
func validateInstrumentations(instCandidates []*current.Instrumentation) error {
	languages := map[string]*current.Instrumentation{}
	dups := map[string][]*current.Instrumentation{}
	for _, candidate := range instCandidates {
		if currentInst, ok := languages[candidate.Spec.Agent.Language]; ok {
			if !currentInst.Spec.Agent.IsEqual(candidate.Spec.Agent) {
				dups[candidate.Spec.Agent.Language] = append(dups[candidate.Spec.Agent.Language], candidate)
			}
		} else {
			languages[candidate.Spec.Agent.Language] = candidate
			dups[candidate.Spec.Agent.Language] = append(dups[candidate.Spec.Agent.Language], candidate)
		}
	}
	var dupIssues []string
	for language, insts := range dups {
		if len(insts) > 1 {
			instNames := make([]string, len(insts))
			for i, inst := range insts {
				instNames[i] = inst.Name
			}
			dupIssues = append(dupIssues, fmt.Sprintf("lang: %s, insts: %v", language, instNames))
		}
	}
	if len(dupIssues) > 0 {
		return fmt.Errorf("%w > (%s)", errMultipleInstancesPossible, strings.Join(dupIssues, "; "))
	}
	return nil
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
func (il *NewrelicInstrumentationLocator) GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error) {
	logger := il.logger.WithValues("namespace", pod.Namespace, "name", pod.Name, "generate_name", pod.GenerateName)

	var listInst current.InstrumentationList
	if err := il.client.List(ctx, &listInst); err != nil {
		return nil, err
	}

	//nolint:prealloc
	var candidates []*current.Instrumentation
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

func filterInstrumentations(insts []*current.Instrumentation, pod *corev1.Pod) map[string][]*current.Instrumentation {
	candidates := map[string][]*current.Instrumentation{}
	for _, inst := range insts {
		instContainerSelector := inst.Spec.ContainerSelector
		if instContainerSelector.IsEmpty() {
			// no containers could match, because we need at least 1 container
			if len(pod.Spec.Containers) == 0 {
				continue
			}
			candidates[pod.Spec.Containers[0].Name] = append(candidates[pod.Spec.Containers[0].Name], inst)
			continue
		}

		matchConstraints := 0

		containerEnvSelector, _ := instContainerSelector.EnvSelector.AsSelector()
		matchConstraints++

		containerImageSelector, _ := instContainerSelector.ImageSelector.AsSelector()
		matchConstraints++

		containerNameSelector, _ := instContainerSelector.NameSelector.AsSelector()
		matchConstraints++

		allContainerNames := map[string]struct{}{}
		for _, container := range pod.Spec.InitContainers {
			allContainerNames[container.Name] = struct{}{}
		}
		for _, container := range pod.Spec.Containers {
			allContainerNames[container.Name] = struct{}{}
		}

		partialMatchingContainers := map[string]int{}

		if inst.Spec.ContainerSelector.NamesFromPodAnnotation != "" {
			matchConstraints++
		}
		if inst.Spec.ContainerSelector.NamesFromPodLabel != "" {
			matchConstraints++
		}

		for _, container := range pod.Spec.InitContainers {
			if containerEnvSelector.Matches(fields.Set(util.EnvToMap(container.Env))) {
				partialMatchingContainers[container.Name]++
			}
			if containerNameSelector.Matches(fields.Set(map[string]string{"initContainer": container.Name, "anyContainer": container.Name})) {
				partialMatchingContainers[container.Name]++
			}
			if containerImageSelector.Matches(fields.Set(util.ParseContainerImage(container.Image))) {
				partialMatchingContainers[container.Name]++
			}
		}
		for _, container := range pod.Spec.Containers {
			if containerEnvSelector.Matches(fields.Set(util.EnvToMap(container.Env))) {
				partialMatchingContainers[container.Name]++
			}
			if containerNameSelector.Matches(fields.Set(map[string]string{"container": container.Name, "anyContainer": container.Name})) {
				partialMatchingContainers[container.Name]++
			}
			if containerImageSelector.Matches(fields.Set(util.ParseContainerImage(container.Image))) {
				partialMatchingContainers[container.Name]++
			}
		}
		if len(inst.Spec.ContainerSelector.NamesFromPodAnnotation) > 0 {
			containerNames, ok := pod.Annotations[inst.Spec.ContainerSelector.NamesFromPodAnnotation]
			if !ok {
				continue
			}
			for _, containerName := range strings.Split(containerNames, ",") {
				if _, ok := allContainerNames[containerName]; ok {
					partialMatchingContainers[containerName]++
				}
			}
		}
		if len(inst.Spec.ContainerSelector.NamesFromPodLabel) > 0 {
			containerNames, ok := pod.Labels[inst.Spec.ContainerSelector.NamesFromPodLabel]
			if !ok {
				continue
			}
			for _, containerName := range strings.Split(containerNames, ",") {
				if _, ok := allContainerNames[containerName]; ok {
					partialMatchingContainers[containerName]++
				}
			}
		}

		var matchingContainers []string
		for name, count := range partialMatchingContainers {
			if count == matchConstraints {
				matchingContainers = append(matchingContainers, name)
			}
		}

		if len(matchingContainers) == 0 {
			continue
		}

		for _, name := range matchingContainers {
			candidates[name] = append(candidates[name], inst)
		}
	}
	return candidates
}

func sortInstrumentations(candidates map[string][]*current.Instrumentation) map[string][]*current.Instrumentation {
	for _, cInsts := range candidates {
		sort.Slice(cInsts, func(i, j int) bool {
			if strings.Compare(cInsts[i].Spec.Agent.Language, cInsts[j].Spec.Agent.Language) > 0 {
				return true
			}
			return false
		})
	}
	return candidates
}

func validateAndReduceInstrumentations(candidates map[string][]*current.Instrumentation) (map[string][]*current.Instrumentation, error) {
	for containerName, cInsts := range candidates {
		if err := validateInstrumentations(cInsts); err != nil {
			return nil, fmt.Errorf("failed to get language instrumentations for container %q: %w", containerName, err)
		}
		candidates[containerName] = reduceIdenticalInstrumentations(cInsts)
	}
	return candidates, nil
}

func validateInstrumentationLicenseKeySecrets(candidates map[string][]*current.Instrumentation) error {
	for containerName, cInsts := range candidates {
		if err := validateLicenseKeySecret(cInsts); err != nil {
			instNames := make([]string, len(cInsts))
			for i, ci := range cInsts {
				instNames[i] = ci.Name
			}
			return fmt.Errorf("failed to get licence key secret name for container %q with instrumentations names: %q: %w", containerName, instNames, err)
		}
	}
	return nil
}

func validateHealthAgents(candidates map[string][]*current.Instrumentation) error {
	healthAgents := 0
	var matchedContainers []string
	for _, cInsts := range candidates {
		for _, inst := range cInsts {
			if !inst.Spec.HealthAgent.IsEmpty() {
				healthAgents++
				matchedContainers = append(matchedContainers, inst.Name)
			}
		}
	}
	if healthAgents > 1 {
		return fmt.Errorf("too many health agents, only 1 per pod on a single agent container is supported; healthAgents: %d, matched containers: %v", healthAgents, matchedContainers)
	}
	return nil
}

func validateContainerWithHealthAgent(candidates map[string][]*current.Instrumentation, pod *corev1.Pod) error {
	if len(pod.Spec.InitContainers) == 0 {
		return nil
	}
	initContainers := map[string]struct{}{}
	for _, c := range pod.Spec.InitContainers {
		initContainers[c.Name] = struct{}{}
	}
	var matchedContainers []string
	for containerName, cInsts := range candidates {
		if _, ok := initContainers[containerName]; !ok {
			continue
		}
		for _, inst := range cInsts {
			if !inst.Spec.HealthAgent.IsEmpty() {
				matchedContainers = append(matchedContainers, inst.Name)
			}
		}
	}
	if len(matchedContainers) > 0 {
		return fmt.Errorf("healthAgent can't be used with initContainers; matched init containers: %v", matchedContainers)
	}
	return nil
}

// validateLicenseKeySecret is used to ensure a secret key name used for the license key is the same for a list of
// Instrumentation's for a single container.
func validateLicenseKeySecret(insts []*current.Instrumentation) error {
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
			return errMultipleSecretsPossible
		}
	}
	return nil
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
	logger := sr.logger.WithValues("namespace", pod.Namespace, "name", pod.Name, "generate_name", pod.GenerateName)

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
		logger.Error(err, "failed to retrieve the secret from operator namespace", "operator_namespace", operatorNamespace, "secret_name", secretName)
		return err
	}

	newSecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns.Name,
			Labels: map[string]string{
				WellKnownLabelManagedBy: ManagedBy,
			},
		},
		Data: secret.Data,
	}
	if err = sr.client.Create(ctx, &newSecret); err != nil {
		logger.Error(err, "failed to create a new secret")
		return err
	}

	return nil
}

func GetLicenseKeySecretsFromInstrumentations(instrumentations map[string][]*current.Instrumentation) []string {
	secrets := map[string]struct{}{}
	for _, insts := range instrumentations {
		for _, inst := range insts {
			secretName := inst.Spec.LicenseKeySecret
			if secretName == "" {
				secretName = DefaultLicenseKeySecretName
			}
			secrets[secretName] = struct{}{}
		}
	}
	secretList := make([]string, 0, len(secrets))
	for secretName := range secrets {
		secretList = append(secretList, secretName)
	}
	return secretList
}

func GetAgentConfigMapsFromInstrumentations(instrumentations map[string][]*current.Instrumentation) []string {
	configMaps := map[string]struct{}{}
	for _, insts := range instrumentations {
		for _, inst := range insts {
			configMapName := inst.Spec.AgentConfigMap
			if configMapName == "" {
				continue
			}
			configMaps[configMapName] = struct{}{}
		}
	}
	if len(configMaps) == 0 {
		return nil
	}
	configMapList := make([]string, 0, len(configMaps))
	for secretName := range configMaps {
		configMapList = append(configMapList, secretName)
	}
	return configMapList
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
			Labels: map[string]string{
				WellKnownLabelManagedBy: ManagedBy,
			},
		},
		Data: configMap.Data,
	}
	if err = cr.client.Create(ctx, &newConfigMap); err != nil {
		logger.Error(err, "failed to create a new config map")
		return err
	}

	return nil
}

// GetConfigMapsFromInstrumentationsAgentEnv to find all the unique configmap names from the agent.env
func GetConfigMapsFromInstrumentationsAgentEnv(instrumentations map[string][]*current.Instrumentation) []string {
	uniqueNames := map[string]struct{}{}
	for _, insts := range instrumentations {
		for _, inst := range insts {
			for _, entry := range inst.Spec.Agent.Env {
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
		}
	}
	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}
	return names
}

// GetSecretsFromInstrumentationsAgentEnv to find all the unique secret names from the agent.env
func GetSecretsFromInstrumentationsAgentEnv(instrumentations map[string][]*current.Instrumentation) []string {
	uniqueNames := map[string]struct{}{}
	for _, insts := range instrumentations {
		for _, inst := range insts {
			for _, entry := range inst.Spec.Agent.Env {
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
		}
	}
	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}
	return names
}

func GetUniqStrings(strs []string) []string {
	uniqueStrs := map[string]struct{}{}
	for _, str := range strs {
		uniqueStrs[str] = struct{}{}
	}
	if len(uniqueStrs) == 0 {
		return nil
	}
	retStrs := make([]string, 0, len(uniqueStrs))
	for str := range uniqueStrs {
		retStrs = append(retStrs, str)
	}
	return retStrs
}
