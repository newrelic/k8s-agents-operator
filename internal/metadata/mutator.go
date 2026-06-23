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
package metadata

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// OptOutAnnotation is the namespace annotation to opt-out of metadata injection
	OptOutAnnotation = "newrelic.com/metadata-injection"

	// Environment variable names for Kubernetes metadata
	EnvClusterName       = "NEW_RELIC_METADATA_KUBERNETES_CLUSTER_NAME"
	EnvNodeName          = "NEW_RELIC_METADATA_KUBERNETES_NODE_NAME"
	EnvNamespaceName     = "NEW_RELIC_METADATA_KUBERNETES_NAMESPACE_NAME"
	EnvPodName           = "NEW_RELIC_METADATA_KUBERNETES_POD_NAME"
	EnvDeploymentName    = "NEW_RELIC_METADATA_KUBERNETES_DEPLOYMENT_NAME"
	EnvContainerName     = "NEW_RELIC_METADATA_KUBERNETES_CONTAINER_NAME"
	EnvContainerImageName = "NEW_RELIC_METADATA_KUBERNETES_CONTAINER_IMAGE_NAME"
)

// MetadataInjectionPodMutator implements webhook.PodMutator for injecting Kubernetes metadata
type MetadataInjectionPodMutator struct {
	Client client.Client
	Config MetadataConfig
	Logger logr.Logger
}

// NewMetadataInjectionPodMutator creates a new metadata injection mutator
func NewMetadataInjectionPodMutator(client client.Client, config MetadataConfig, logger logr.Logger) *MetadataInjectionPodMutator {
	return &MetadataInjectionPodMutator{
		Client: client,
		Config: config,
		Logger: logger,
	}
}

// Mutate implements the PodMutator interface to inject Kubernetes metadata into pod containers
func (m *MetadataInjectionPodMutator) Mutate(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	logger := m.Logger.WithValues("namespace", ns.Name, "pod", pod.Name)

	// Check if namespace should be skipped
	if m.shouldSkipNamespace(ns) {
		logger.V(1).Info("skipping metadata injection for namespace")
		return pod, nil
	}

	logger.Info("injecting Kubernetes metadata into pod")

	// Inject metadata into all regular containers
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		m.injectMetadataEnvVars(container, &pod, ns.Name)
	}

	// Inject metadata into all init containers
	for i := range pod.Spec.InitContainers {
		container := &pod.Spec.InitContainers[i]
		m.injectMetadataEnvVars(container, &pod, ns.Name)
	}

	return pod, nil
}

// shouldSkipNamespace determines if metadata injection should be skipped for a namespace
func (m *MetadataInjectionPodMutator) shouldSkipNamespace(ns corev1.Namespace) bool {
	// Check if namespace is in the ignore list
	if m.Config.IsNamespaceIgnored(ns.Name) {
		return true
	}

	// Check for opt-out annotation
	if ns.Annotations != nil {
		if optOut, exists := ns.Annotations[OptOutAnnotation]; exists {
			// Opt-out if annotation value is explicitly "false" or "disabled"
			if optOut == "false" || optOut == "disabled" {
				return true
			}
		}
	}

	// If namespace label selector is configured, check if namespace matches
	if m.Config.NamespaceLabelSelector != "" {
		selector, err := labels.Parse(m.Config.NamespaceLabelSelector)
		if err != nil {
			m.Logger.Error(err, "failed to parse namespace label selector", "selector", m.Config.NamespaceLabelSelector)
			// On parse error, default to not skipping (fail open)
			return false
		}

		// Skip if namespace labels don't match the selector
		if !selector.Matches(labels.Set(ns.Labels)) {
			return true
		}
	}

	return false
}

// injectMetadataEnvVars injects Kubernetes metadata environment variables into a container
func (m *MetadataInjectionPodMutator) injectMetadataEnvVars(container *corev1.Container, pod *corev1.Pod, namespaceName string) {
	// 1. Cluster name (static value from config)
	if m.Config.ClusterName != "" {
		setEnvVarIfNotExists(container, EnvClusterName, corev1.EnvVar{
			Name:  EnvClusterName,
			Value: m.Config.ClusterName,
		})
	}

	// 2. Node name (downward API)
	setEnvVarIfNotExists(container, EnvNodeName, corev1.EnvVar{
		Name: EnvNodeName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "spec.nodeName",
			},
		},
	})

	// 3. Namespace name (downward API)
	setEnvVarIfNotExists(container, EnvNamespaceName, corev1.EnvVar{
		Name: EnvNamespaceName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.namespace",
			},
		},
	})

	// 4. Pod name (downward API)
	setEnvVarIfNotExists(container, EnvPodName, corev1.EnvVar{
		Name: EnvPodName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.name",
			},
		},
	})

	// 5. Deployment name (derived from pod metadata)
	deploymentName := deriveDeploymentName(pod)
	if deploymentName != "" {
		setEnvVarIfNotExists(container, EnvDeploymentName, corev1.EnvVar{
			Name:  EnvDeploymentName,
			Value: deploymentName,
		})
	}

	// 6. Container name (static value)
	setEnvVarIfNotExists(container, EnvContainerName, corev1.EnvVar{
		Name:  EnvContainerName,
		Value: container.Name,
	})

	// 7. Container image name (static value)
	setEnvVarIfNotExists(container, EnvContainerImageName, corev1.EnvVar{
		Name:  EnvContainerImageName,
		Value: container.Image,
	})
}

// deriveDeploymentName attempts to derive the deployment name from pod metadata
// This follows the k8s-metadata-injection heuristic:
// - If pod has exactly one owner reference with Kind="ReplicaSet"
// - Strip the last 2 segments from pod.GenerateName
// - Otherwise return empty string
func deriveDeploymentName(pod *corev1.Pod) string {
	// Check if pod has owner references
	if len(pod.OwnerReferences) != 1 {
		return ""
	}

	// Check if the owner is a ReplicaSet
	owner := pod.OwnerReferences[0]
	if owner.Kind != "ReplicaSet" {
		return ""
	}

	// Extract deployment name from pod's GenerateName
	// Example: "myapp-7f9c4d-" → "myapp"
	generateName := pod.GenerateName
	if generateName == "" {
		return ""
	}

	// Remove trailing dash
	generateName = strings.TrimSuffix(generateName, "-")

	// Split by dash and remove last 2 segments (replicaset hash and random suffix)
	parts := strings.Split(generateName, "-")
	if len(parts) < 2 {
		return ""
	}

	// Remove last 2 segments
	deploymentParts := parts[:len(parts)-1]

	return strings.Join(deploymentParts, "-")
}

// setEnvVarIfNotExists adds an environment variable to a container if it doesn't already exist
// This ensures idempotency - existing env vars are never overwritten
func setEnvVarIfNotExists(container *corev1.Container, envVarName string, envVar corev1.EnvVar) {
	// Check if env var already exists
	for _, existingEnv := range container.Env {
		if existingEnv.Name == envVarName {
			// Env var already exists, don't override
			return
		}
	}

	// Add the new env var
	container.Env = append(container.Env, envVar)
}

// isOwnedByKind checks if a pod is owned by a specific Kubernetes kind
func isOwnedByKind(pod *corev1.Pod, kind string) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == kind {
			return true
		}
	}
	return false
}

// getOwnerByKind returns the first owner reference of a specific kind
func getOwnerByKind(pod *corev1.Pod, kind string) *metav1.OwnerReference {
	for i := range pod.OwnerReferences {
		if pod.OwnerReferences[i].Kind == kind {
			return &pod.OwnerReferences[i]
		}
	}
	return nil
}
