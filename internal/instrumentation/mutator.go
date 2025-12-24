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
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/selector"
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
	errMultipleInstancesPossible  = errors.New("multiple New Relic Instrumentation instances available, cannot determine which one to select")
	errNoInstancesAvailable       = errors.New("no New Relic Instrumentation instances available")
	errMultipleSecretsPossible    = errors.New("multiple license key secrets possible")
	errMultipleConfigMapsPossible = errors.New("multiple agent configmaps possible")
)

type InstrumentationPodMutator struct {
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
	client client.Client,
	sdkInjector SdkContainerInjector,
	secretReplicator SecretsReplicator,
	configMapReplicator ConfigMapsReplicator,
	instrumentationLocator InstrumentationLocator,
	operatorNamespace string,
) *InstrumentationPodMutator {
	return &InstrumentationPodMutator{
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
//
//nolint:gocyclo
func (pm *InstrumentationPodMutator) Mutate(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	logger, _ := logr.FromContext(ctx)

	instCandidates, err := pm.instrumentationLocator.GetInstrumentations(ctx, ns, pod)
	if err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a instrumentation instance for this pod")
		return corev1.Pod{}, err
	}
	if len(instCandidates) == 0 {
		logger.Info("no instrumentation instances for this pod")
		return corev1.Pod{}, errNoInstancesAvailable
	}

	logger.Info("instrumentation instances found matching pod", "count", len(instCandidates), "candidates", getInstrumentationNamesFromList(instCandidates))

	// filter by container name; map[container.Name]=[instrumentation, instrumentations...]
	instrumentations := filterInstrumentations(ctx, instCandidates, &pod)

	// sort, so that, given the same list of instrumentations, we always apply the mutations in the same predictable order
	instrumentations = sortInstrumentations(instrumentations)

	// remove duplicate instrumentations by language (if identical, otherwise error)
	instrumentations, err = validateAndReduceInstrumentations(instrumentations)
	if err != nil {
		if errors.Is(err, errMultipleInstancesPossible) {
			logger.Info("too many instrumentation instances for the pod container")
		} else {
			logger.Error(err, "failed to select a instrumentation instance for this pod")
		}
		return corev1.Pod{}, err
	}

	c := countInstrumentationsInMap(instrumentations)
	if c == 0 {
		logger.Info("no instrumentation instances for this pod matched any containers")
		// nothing matched
		return pod, nil
	}

	logger.Info("matched instrumentation instances for this pods containers",
		"count", c, "matches", getInstrumentationNamesInMapAsMap(instrumentations))

	// ensure we have only one license key per container
	if err = validateInstrumentationLicenseKeySecrets(instrumentations); err != nil {
		logger.Error(err, "failed to select a instrumentation instance for this pod")
		return corev1.Pod{}, err
	}

	// ensure we have only one agent configmap per container
	if err = validateInstrumentationAgentConfigMaps(instrumentations); err != nil {
		logger.Error(err, "failed to select a instrumentation instance for this pod")
		return corev1.Pod{}, err
	}

	// ensure we have only one agent configmap per container
	if err = validateContainerInstrumentationsForConflictingChanges(instrumentations); err != nil {
		logger.Error(err, "failed to select a instrumentation instance for this pod")
		return corev1.Pod{}, err
	}

	// ensure we only have 1 health agent
	if err = validateHealthAgents(instrumentations); err != nil {
		logger.Error(err, "failed to select a instrumentation instance for this pod")
		return corev1.Pod{}, err
	}

	// ensure that the health sidecar is not being used with an init container
	if err = validateContainerWithHealthAgent(instrumentations, &pod); err != nil {
		logger.Error(err, "failed to select a instrumentation instance for this pod")
		return corev1.Pod{}, err
	}

	pm.replicateSecrets(ctx, ns, pod, instrumentations)
	pm.replicateConfigMaps(ctx, ns, pod, instrumentations)

	return pm.sdkInjector.InjectContainers(ctx, instrumentations, ns, pod), nil
}

func getInstrumentationNamesFromList(instrumentations []*current.Instrumentation) []string {
	names := make([]string, len(instrumentations))
	for i, candidate := range instrumentations {
		names[i] = candidate.Name
	}
	return names
}

func getInstrumentationNamesInMapAsMap(instrumentations map[string][]*current.Instrumentation) map[string][]string {
	matches := map[string][]string{}
	for name, insts := range instrumentations {
		names := make([]string, len(insts))
		for i, inst := range insts {
			names[i] = inst.Name
		}
		matches[name] = names
	}
	return matches
}

func countInstrumentationsInMap(instrumentations map[string][]*current.Instrumentation) int {
	c := 0
	for _, insts := range instrumentations {
		c += len(insts)
	}
	return c
}

func (pm *InstrumentationPodMutator) replicateSecrets(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, instrumentations map[string][]*current.Instrumentation) {
	logger, _ := logr.FromContext(ctx)
	if pm.secretReplicationIsEnabled {
		secrets := GetLicenseKeySecretsFromInstrumentations(instrumentations)
		if pm.envSecretReplicationIsEnabled {
			envSecrets := GetSecretsFromInstrumentationsAgentEnv(instrumentations)
			secrets = append(secrets, envSecrets...)
		}
		secrets = GetUniqStrings(secrets)
		if err := pm.secretReplicator.ReplicateSecrets(ctx, ns, pod, pm.operatorNamespace, secrets); err != nil {
			logger.Error(err, "failed to replicate secrets")
		}
	}
}

func (pm *InstrumentationPodMutator) replicateConfigMaps(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, instrumentations map[string][]*current.Instrumentation) {
	logger, _ := logr.FromContext(ctx)
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
		if err := pm.configMapReplicator.ReplicateConfigMaps(ctx, ns, pod, pm.operatorNamespace, configMaps); err != nil {
			logger.Error(err, "failed to replicate config maps")
		}
	}
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

var langRoots = map[string]string{
	"dotnet":             "dotnet",
	"dotnet-windows2022": "dotnet",
	"dotnet-windows2025": "dotnet",
	"java":               "java",
	"nodejs":             "nodejs",
	"php-7.2":            "php",
	"php-7.3":            "php",
	"php-7.4":            "php",
	"php-8.0":            "php",
	"php-8.1":            "php",
	"php-8.2":            "php",
	"php-8.3":            "php",
	"php-8.4":            "php",
	"php-8.5":            "php",
	"python":             "python",
	"ruby":               "ruby",
}

func agentIsEqual(a, b current.Agent) bool {
	return a.Image == b.Image && reflect.DeepEqual(a.Env, b.Env) &&
		reflect.DeepEqual(a.VolumeSizeLimit, b.VolumeSizeLimit) &&
		reflect.DeepEqual(a.Resources, b.Resources) &&
		reflect.DeepEqual(a.ImagePullPolicy, b.ImagePullPolicy) &&
		reflect.DeepEqual(a.SecurityContext, b.SecurityContext)
}

// validateInstrumentations is used to validate that only a single instrumentation exists for each language
func validateInstrumentations(instCandidates []*current.Instrumentation) error {
	languages := map[string]*current.Instrumentation{}

	dups := map[string][]*current.Instrumentation{}
	for _, candidate := range instCandidates {
		candidateLang := langRoots[candidate.Spec.Agent.Language]
		if currentInst, ok := languages[candidateLang]; ok {
			if !agentIsEqual(currentInst.Spec.Agent, candidate.Spec.Agent) {
				dups[candidateLang] = append(dups[candidateLang], candidate)
			}
		} else {
			languages[candidateLang] = candidate
			dups[candidateLang] = append(dups[candidateLang], candidate)
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
	client            client.Client
	operatorNamespace string
}

// NewNewRelicInstrumentationLocator is the constructor for getting instrumentations
func NewNewRelicInstrumentationLocator(client client.Client, operatorNamespace string) *NewrelicInstrumentationLocator {
	return &NewrelicInstrumentationLocator{
		client:            client,
		operatorNamespace: operatorNamespace,
	}
}

// GetInstrumentations is used to get all instrumentations in the cluster. While we could limit it to the operator
// namespace, it's more helpful to list anything in the logs that may have been excluded.
func (il *NewrelicInstrumentationLocator) GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error) {
	logger, _ := logr.FromContext(ctx)

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

		if !podSelector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		if !namespaceSelector.Matches(labels.Set(ns.Labels)) {
			continue
		}

		if inst.Spec.LicenseKeySecret == "" {
			inst.Spec.LicenseKeySecret = DefaultLicenseKeySecretName
		}
		candidates = append(candidates, &inst)
	}
	return candidates, nil
}

func filterInstrumentations(ctx context.Context, insts []*current.Instrumentation, pod *corev1.Pod) map[string][]*current.Instrumentation {
	logger, _ := logr.FromContext(ctx)
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

		containerEnvSelector, _ := instContainerSelector.EnvSelector.AsSelector()
		containerImageSelector, _ := instContainerSelector.ImageSelector.AsSelector()
		containerNameSelector, _ := instContainerSelector.NameSelector.AsSelector()

		matchingSelectors := map[string]map[string]struct{}{}
		matchingSelectors["envSelector"] = getContainerSetByEnvSelector(containerEnvSelector, pod)
		matchingSelectors["nameSelector"] = getContainerSetByNameSelector(containerNameSelector, pod)
		matchingSelectors["imageSelector"] = getContainerSetByImageSelector(containerImageSelector, pod)
		matchingSelectors["namesFromPodAnnotation"] = getContainerSetByNamesFromPodAnnotations(inst.Spec.ContainerSelector.NamesFromPodAnnotation, pod)

		currentSet := getContainerNamesSetFromPod(pod)
		for _, set := range matchingSelectors {
			currentSet = intersectSet(set, currentSet)
		}
		matchingContainers := setToList(currentSet)

		logger.Info("matched containers",
			"instrumentation_name", inst.Name,
			"containers", matchingContainers,
		)

		if len(matchingContainers) == 0 {
			continue
		}

		for _, name := range matchingContainers {
			candidates[name] = append(candidates[name], inst)
		}
	}
	return candidates
}

func intersectSet(a, b map[string]struct{}) map[string]struct{} {
	if a == nil || b == nil {
		return nil
	}
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	if len(a) > len(b) {
		a, b = b, a
	}
	for k := range a {
		if _, ok := b[k]; ok {
			set[k] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func setToList(s map[string]struct{}) []string {
	l := make([]string, 0, len(s))
	for k := range s {
		l = append(l, k)
	}
	slices.Sort(l)
	return l
}

func getContainerSetByEnvSelector(containerEnvSelector selector.Selector, pod *corev1.Pod) map[string]struct{} {
	matchingContainers := map[string]struct{}{}
	for _, container := range pod.Spec.InitContainers {
		if containerEnvSelector.Matches(util.EnvToMap(container.Env)) {
			matchingContainers[container.Name] = struct{}{}
		}
	}
	for _, container := range pod.Spec.Containers {
		if containerEnvSelector.Matches(util.EnvToMap(container.Env)) {
			matchingContainers[container.Name] = struct{}{}
		}
	}
	return matchingContainers
}

func getContainerSetByNameSelector(containerNameSelector selector.Selector, pod *corev1.Pod) map[string]struct{} {
	matchingContainers := map[string]struct{}{}
	for _, container := range pod.Spec.InitContainers {
		if containerNameSelector.Matches(map[string]string{"initContainer": container.Name, "anyContainer": container.Name}) {
			matchingContainers[container.Name] = struct{}{}
		}
	}
	for _, container := range pod.Spec.Containers {
		if containerNameSelector.Matches(map[string]string{"container": container.Name, "anyContainer": container.Name}) {
			matchingContainers[container.Name] = struct{}{}
		}
	}
	return matchingContainers
}

func getContainerSetByImageSelector(containerImageSelector selector.Selector, pod *corev1.Pod) map[string]struct{} {
	matchingContainers := map[string]struct{}{}
	for _, container := range pod.Spec.InitContainers {
		if containerImageSelector.Matches(util.ParseContainerImage(container.Image)) {
			matchingContainers[container.Name] = struct{}{}
		}
	}
	for _, container := range pod.Spec.Containers {
		if containerImageSelector.Matches(util.ParseContainerImage(container.Image)) {
			matchingContainers[container.Name] = struct{}{}
		}
	}
	return matchingContainers
}

func getContainerNamesSetFromPod(pod *corev1.Pod) map[string]struct{} {
	allContainerNames := map[string]struct{}{}
	for _, container := range pod.Spec.InitContainers {
		allContainerNames[container.Name] = struct{}{}
	}
	for _, container := range pod.Spec.Containers {
		allContainerNames[container.Name] = struct{}{}
	}
	return allContainerNames
}

func getContainerSetByNamesFromPodAnnotations(annotationKey string, pod *corev1.Pod) map[string]struct{} {
	allContainerNames := getContainerNamesSetFromPod(pod)
	if len(annotationKey) == 0 {
		return allContainerNames
	}
	containerNames, ok := pod.Annotations[annotationKey]
	if !ok {
		return allContainerNames
	}
	matchingContainers := map[string]struct{}{}
	for _, containerName := range strings.Split(containerNames, ",") {
		if _, ok := allContainerNames[containerName]; ok {
			matchingContainers[containerName] = struct{}{}
		}
	}
	return matchingContainers
}

func sortInstrumentations(candidates map[string][]*current.Instrumentation) map[string][]*current.Instrumentation {
	for _, cInsts := range candidates {
		sort.Slice(cInsts, func(i, j int) bool {
			return strings.Compare(cInsts[i].Spec.Agent.Language, cInsts[j].Spec.Agent.Language) > 0
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
			return fmt.Errorf("failed to get license key secret name for container %q with instrumentations names: %q: %w", containerName, instNames, err)
		}
	}
	return nil
}

func validateContainerInstrumentationsForConflictingChanges(candidates map[string][]*current.Instrumentation) error {
	for containerName, cInsts := range candidates {
		if err := validateContainerInstrumentations(cInsts); err != nil {
			return fmt.Errorf("failed to validate that no conflicting changes occurred for container %q: %w", containerName, err)
		}
	}
	return nil
}

//nolint:gocyclo
func validateContainerInstrumentations(instCandidates []*current.Instrumentation) error {
	var res *current.Instrumentation
	var errs []error

	var instsWithAgentConfigMap []string
	var instsWithNewRelicFile []string

	for _, candidate := range instCandidates {
		if candidate.Spec.Agent.Language == "java" && candidate.Spec.AgentConfigMap != "" {
			instsWithAgentConfigMap = append(instsWithAgentConfigMap, candidate.Name)
		}
		for _, entry := range candidate.Spec.Agent.Env {
			if entry.Name == "NEWRELIC_FILE" {
				instsWithNewRelicFile = append(instsWithNewRelicFile, candidate.Name)
				break
			}
		}
	}
	if len(instsWithAgentConfigMap) > 0 && len(instsWithNewRelicFile) > 0 {
		errs = append(errs, fmt.Errorf("agentConfigMap and env.name == NEWRELIC_FILE can't be used together; conflicting instrumentations: %v", instsWithNewRelicFile))
	}

	for _, candidate := range instCandidates {
		if res == nil {
			res = candidate
			continue
		}

		if candidate.Spec.AgentConfigMap != res.Spec.AgentConfigMap {
			errs = append(errs, fmt.Errorf("instrumentation.spec.agentConfigMap conflicts with multiple instrumentations matching the same container; instrumentations: %s, %s", res.Name, candidate.Name))
			continue
		}
		if candidate.Spec.LicenseKeySecret != res.Spec.LicenseKeySecret {
			errs = append(errs, fmt.Errorf("instrumentation.spec.agent.licenseKeySecret conflicts with multiple instrumentations matching the same container; instrumentations: %s, %s", res.Name, candidate.Name))
			continue
		}

		if !reflect.DeepEqual(candidate.Spec.Agent.Resources, res.Spec.Agent.Resources) {
			errs = append(errs, fmt.Errorf("instrumentation.spec.agent.resourceRequirements conflicts with multiple instrumentations matching the same container; instrumentations: %s, %s", res.Name, candidate.Name))
			continue
		}
		if !reflect.DeepEqual(candidate.Spec.Agent.SecurityContext, res.Spec.Agent.SecurityContext) {
			errs = append(errs, fmt.Errorf("instrumentation.spec.agent.securityContext conflicts with multiple instrumentations matching the same container; instrumentations: %s, %s", res.Name, candidate.Name))
			continue
		}
		if !reflect.DeepEqual(candidate.Spec.Agent.Env, res.Spec.Agent.Env) {
			candidateEnv := map[string]*corev1.EnvVar{}
			for _, entry := range candidate.Spec.Agent.Env {
				candidateEnv[entry.Name] = &entry
			}
			var unequalEntryNames []string
			for _, entry := range res.Spec.Agent.Env {
				candidateEnvEntry, ok := candidateEnv[entry.Name]
				if !ok {
					continue
				}
				if !reflect.DeepEqual(candidateEnvEntry, &entry) {
					unequalEntryNames = append(unequalEntryNames, entry.Name)
				}
			}
			if len(unequalEntryNames) > 0 {
				errs = append(errs, fmt.Errorf("instrumentation.spec.agent.env with entry keys %v, conflicts with multiple instrumentations matching the same container; instrumentations: %s, %s", unequalEntryNames, res.Name, candidate.Name))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func validateInstrumentationAgentConfigMaps(candidates map[string][]*current.Instrumentation) error {
	for containerName, cInsts := range candidates {
		if err := validateAgentConfigMap(cInsts); err != nil {
			instNames := make([]string, len(cInsts))
			for i, ci := range cInsts {
				instNames[i] = ci.Name
			}
			return fmt.Errorf("failed to get agent configmap name for container %q with instrumentations names: %q: %w", containerName, instNames, err)
		}
	}
	return nil
}

func validateHealthAgents(candidates map[string][]*current.Instrumentation) error {
	for _, cInsts := range candidates {
		for _, inst := range cInsts {
			if !inst.Spec.HealthAgent.IsEmpty() && len(cInsts) > 1 {
				return fmt.Errorf("only one agent can be injected in a container if a health agent is used")
			}
		}
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

func validateAgentConfigMap(insts []*current.Instrumentation) error {
	var agentConfigMap *string
	for _, inst := range insts {
		specAgentConfigMap := inst.Spec.AgentConfigMap
		if agentConfigMap == nil && specAgentConfigMap != "" {
			agentConfigMap = &specAgentConfigMap
			continue
		}
		if agentConfigMap == nil {
			continue
		}
		if *agentConfigMap != specAgentConfigMap {
			return errMultipleConfigMapsPossible
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
func NewNewrelicSecretReplicator(client client.Client) *NewrelicSecretReplicator {
	return &NewrelicSecretReplicator{client: client}
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
	logger, _ := logr.FromContext(ctx)

	var secret corev1.Secret

	if secretName == "" {
		secretName = DefaultLicenseKeySecretName
	}

	err := sr.client.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: secretName}, &secret)
	if err == nil {
		logger.V(1).Info("secret already exists")
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
}

// NewNewrelicConfigMapReplicator is the constructor for copying configmaps
func NewNewrelicConfigMapReplicator(client client.Client) *NewrelicConfigMapReplicator {
	return &NewrelicConfigMapReplicator{client: client}
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
	logger, _ := logr.FromContext(ctx)
	var configMap corev1.ConfigMap

	if configMapName == "" {
		return fmt.Errorf("config map name required")
	}

	err := cr.client.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: configMapName}, &configMap)
	if err == nil {
		logger.V(1).Info("config map already exists")
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
			for _, entries := range [][]corev1.EnvVar{inst.Spec.Agent.Env, inst.Spec.HealthAgent.Env} {
				for _, entry := range entries {
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
			for _, entries := range [][]corev1.EnvVar{inst.Spec.Agent.Env, inst.Spec.HealthAgent.Env} {
				for _, entry := range entries {
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
