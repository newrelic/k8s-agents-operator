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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMetadataInjectionPodMutator_Mutate(t *testing.T) {
	tests := []struct {
		name           string
		config         MetadataConfig
		namespace      corev1.Namespace
		pod            corev1.Pod
		expectedEnvs   int // number of expected metadata env vars per container
		shouldInject   bool
		checkDeployment bool
		expectedDeployment string
	}{
		{
			name: "basic injection with all variables",
			config: MetadataConfig{
				Enabled:           true,
				ClusterName:       "test-cluster",
				IgnoredNamespaces: []string{"kube-system"},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:         "test-pod",
					GenerateName: "test-deployment-abc-xyz-",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "test-deployment-abc",
						},
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectedEnvs: 7,
			shouldInject: true,
			checkDeployment: true,
			expectedDeployment: "test-deployment-abc",
		},
		{
			name: "skip ignored namespace",
			config: MetadataConfig{
				Enabled:           true,
				ClusterName:       "test-cluster",
				IgnoredNamespaces: []string{"kube-system", "kube-public"},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectedEnvs: 0,
			shouldInject: false,
		},
		{
			name: "skip namespace with opt-out annotation",
			config: MetadataConfig{
				Enabled:           true,
				ClusterName:       "test-cluster",
				IgnoredNamespaces: []string{"kube-system"},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Annotations: map[string]string{
						OptOutAnnotation: "false",
					},
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectedEnvs: 0,
			shouldInject: false,
		},
		{
			name: "skip namespace with opt-out annotation (disabled)",
			config: MetadataConfig{
				Enabled:           true,
				ClusterName:       "test-cluster",
				IgnoredNamespaces: []string{},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Annotations: map[string]string{
						OptOutAnnotation: "disabled",
					},
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectedEnvs: 0,
			shouldInject: false,
		},
		{
			name: "inject when label selector matches",
			config: MetadataConfig{
				Enabled:                true,
				ClusterName:            "test-cluster",
				IgnoredNamespaces:      []string{},
				NamespaceLabelSelector: "inject=enabled",
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"inject": "enabled",
					},
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectedEnvs: 6, // 6 because no deployment name (no owner references)
			shouldInject: true,
		},
		{
			name: "skip when label selector doesn't match",
			config: MetadataConfig{
				Enabled:                true,
				ClusterName:            "test-cluster",
				IgnoredNamespaces:      []string{},
				NamespaceLabelSelector: "inject=enabled",
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"inject": "disabled",
					},
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectedEnvs: 0,
			shouldInject: false,
		},
		{
			name: "inject into multiple containers",
			config: MetadataConfig{
				Enabled:           true,
				ClusterName:       "test-cluster",
				IgnoredNamespaces: []string{},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
						{
							Name:  "sidecar",
							Image: "busybox:latest",
						},
					},
				},
			},
			expectedEnvs: 6, // 6 because no deployment name
			shouldInject: true,
		},
		{
			name: "inject into init containers",
			config: MetadataConfig{
				Enabled:           true,
				ClusterName:       "test-cluster",
				IgnoredNamespaces: []string{},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "init",
							Image: "busybox:latest",
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectedEnvs: 6, // 6 because no deployment name
			shouldInject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().Build()
			logger := logr.Discard()

			mutator := NewMetadataInjectionPodMutator(client, tt.config, logger)

			mutatedPod, err := mutator.Mutate(context.Background(), tt.namespace, tt.pod)
			require.NoError(t, err)

			if !tt.shouldInject {
				// Verify no env vars were added
				for _, container := range mutatedPod.Spec.Containers {
					assert.Equal(t, 0, len(container.Env), "container %s should have no env vars", container.Name)
				}
				return
			}

			// Verify env vars were injected into all containers
			for _, container := range mutatedPod.Spec.Containers {
				assert.GreaterOrEqual(t, len(container.Env), tt.expectedEnvs,
					"container %s should have at least %d env vars", container.Name, tt.expectedEnvs)

				// Verify specific env vars
				if tt.config.ClusterName != "" {
					assert.True(t, hasEnvVar(container.Env, EnvClusterName))
					assert.Equal(t, tt.config.ClusterName, getEnvVarValue(container.Env, EnvClusterName))
				}
				assert.True(t, hasEnvVar(container.Env, EnvNodeName))
				assert.True(t, hasEnvVar(container.Env, EnvNamespaceName))
				assert.True(t, hasEnvVar(container.Env, EnvPodName))
				assert.True(t, hasEnvVar(container.Env, EnvContainerName))
				assert.Equal(t, container.Name, getEnvVarValue(container.Env, EnvContainerName))
				assert.True(t, hasEnvVar(container.Env, EnvContainerImageName))
				assert.Equal(t, container.Image, getEnvVarValue(container.Env, EnvContainerImageName))

				if tt.checkDeployment {
					assert.True(t, hasEnvVar(container.Env, EnvDeploymentName))
					assert.Equal(t, tt.expectedDeployment, getEnvVarValue(container.Env, EnvDeploymentName))
				}

				// Verify downward API env vars
				assertEnvVarUsesFieldRef(t, container.Env, EnvNodeName, "spec.nodeName")
				assertEnvVarUsesFieldRef(t, container.Env, EnvNamespaceName, "metadata.namespace")
				assertEnvVarUsesFieldRef(t, container.Env, EnvPodName, "metadata.name")
			}

			// Verify init containers also get env vars
			for _, container := range mutatedPod.Spec.InitContainers {
				assert.GreaterOrEqual(t, len(container.Env), tt.expectedEnvs,
					"init container %s should have at least %d env vars", container.Name, tt.expectedEnvs)
			}
		})
	}
}

func TestDeriveDeploymentName(t *testing.T) {
	tests := []struct {
		name         string
		pod          corev1.Pod
		expectedName string
	}{
		{
			name: "deployment via replicaset",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "myapp-7f9c4d-",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "myapp-7f9c4d",
						},
					},
				},
			},
			expectedName: "myapp",
		},
		{
			name: "deployment with long name",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "my-long-deployment-name-abc123-xyz456-",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "my-long-deployment-name-abc123",
						},
					},
				},
			},
			expectedName: "my-long-deployment-name-abc123",
		},
		{
			name: "no owner references",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "myapp-",
				},
			},
			expectedName: "",
		},
		{
			name: "multiple owner references",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "myapp-7f9c4d-",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "myapp-7f9c4d",
						},
						{
							Kind: "Other",
							Name: "other",
						},
					},
				},
			},
			expectedName: "",
		},
		{
			name: "non-replicaset owner",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "myapp-0-",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "StatefulSet",
							Name: "myapp",
						},
					},
				},
			},
			expectedName: "",
		},
		{
			name: "no generate name",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myapp-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "myapp-7f9c4d",
						},
					},
				},
			},
			expectedName: "",
		},
		{
			name: "short generate name",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "a-",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "a",
						},
					},
				},
			},
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveDeploymentName(&tt.pod)
			assert.Equal(t, tt.expectedName, result)
		})
	}
}

func TestSetEnvVarIfNotExists_Idempotency(t *testing.T) {
	container := &corev1.Container{
		Name:  "test",
		Image: "nginx",
		Env: []corev1.EnvVar{
			{
				Name:  EnvClusterName,
				Value: "existing-cluster",
			},
		},
	}

	// Try to add the same env var with a different value
	setEnvVarIfNotExists(container, EnvClusterName, corev1.EnvVar{
		Name:  EnvClusterName,
		Value: "new-cluster",
	})

	// Verify the original value is preserved
	assert.Equal(t, 1, len(container.Env))
	assert.Equal(t, "existing-cluster", container.Env[0].Value)

	// Add a new env var
	setEnvVarIfNotExists(container, EnvNodeName, corev1.EnvVar{
		Name: EnvNodeName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "spec.nodeName",
			},
		},
	})

	// Verify the new env var was added
	assert.Equal(t, 2, len(container.Env))
	assert.True(t, hasEnvVar(container.Env, EnvNodeName))
}

func TestShouldSkipNamespace(t *testing.T) {
	tests := []struct {
		name           string
		config         MetadataConfig
		namespace      corev1.Namespace
		expectedResult bool
	}{
		{
			name: "skip ignored namespace",
			config: MetadataConfig{
				Enabled:           true,
				IgnoredNamespaces: []string{"kube-system", "kube-public"},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
				},
			},
			expectedResult: true,
		},
		{
			name: "don't skip non-ignored namespace",
			config: MetadataConfig{
				Enabled:           true,
				IgnoredNamespaces: []string{"kube-system"},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			expectedResult: false,
		},
		{
			name: "skip namespace with opt-out annotation (false)",
			config: MetadataConfig{
				Enabled:           true,
				IgnoredNamespaces: []string{},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Annotations: map[string]string{
						OptOutAnnotation: "false",
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "skip namespace with opt-out annotation (disabled)",
			config: MetadataConfig{
				Enabled:           true,
				IgnoredNamespaces: []string{},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Annotations: map[string]string{
						OptOutAnnotation: "disabled",
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "don't skip namespace with other annotation values",
			config: MetadataConfig{
				Enabled:           true,
				IgnoredNamespaces: []string{},
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Annotations: map[string]string{
						OptOutAnnotation: "true",
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "skip when label selector doesn't match",
			config: MetadataConfig{
				Enabled:                true,
				IgnoredNamespaces:      []string{},
				NamespaceLabelSelector: "inject=enabled",
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"inject": "disabled",
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "don't skip when label selector matches",
			config: MetadataConfig{
				Enabled:                true,
				IgnoredNamespaces:      []string{},
				NamespaceLabelSelector: "inject=enabled",
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"inject": "enabled",
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "don't skip when no label selector",
			config: MetadataConfig{
				Enabled:                true,
				IgnoredNamespaces:      []string{},
				NamespaceLabelSelector: "",
			},
			namespace: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().Build()
			logger := logr.Discard()
			mutator := NewMetadataInjectionPodMutator(client, tt.config, logger)

			result := mutator.shouldSkipNamespace(tt.namespace)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// Helper functions

func hasEnvVar(envVars []corev1.EnvVar, name string) bool {
	for _, env := range envVars {
		if env.Name == name {
			return true
		}
	}
	return false
}

func getEnvVarValue(envVars []corev1.EnvVar, name string) string {
	for _, env := range envVars {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

func assertEnvVarUsesFieldRef(t *testing.T, envVars []corev1.EnvVar, name string, fieldPath string) {
	for _, env := range envVars {
		if env.Name == name {
			require.NotNil(t, env.ValueFrom, "env var %s should use ValueFrom", name)
			require.NotNil(t, env.ValueFrom.FieldRef, "env var %s should use FieldRef", name)
			assert.Equal(t, fieldPath, env.ValueFrom.FieldRef.FieldPath)
			return
		}
	}
	t.Errorf("env var %s not found", name)
}
