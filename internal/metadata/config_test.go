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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		validate    func(t *testing.T, config MetadataConfig)
	}{
		{
			name: "valid configuration with all fields",
			envVars: map[string]string{
				"METADATA_INJECTION_ENABLED":                    "true",
				"K8S_CLUSTER_NAME":                              "production-cluster",
				"METADATA_INJECTION_IGNORED_NAMESPACES":         "kube-system,kube-public,custom-ns",
				"METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR":  "inject=enabled",
			},
			expectError: false,
			validate: func(t *testing.T, config MetadataConfig) {
				assert.True(t, config.Enabled)
				assert.Equal(t, "production-cluster", config.ClusterName)
				assert.Equal(t, []string{"kube-system", "kube-public", "custom-ns"}, config.IgnoredNamespaces)
				assert.Equal(t, "inject=enabled", config.NamespaceLabelSelector)
			},
		},
		{
			name: "disabled metadata injection",
			envVars: map[string]string{
				"METADATA_INJECTION_ENABLED": "false",
			},
			expectError: false,
			validate: func(t *testing.T, config MetadataConfig) {
				assert.False(t, config.Enabled)
			},
		},
		{
			name: "enabled without cluster name - should fail validation",
			envVars: map[string]string{
				"METADATA_INJECTION_ENABLED": "true",
				"K8S_CLUSTER_NAME":           "",
			},
			expectError: true,
		},
		{
			name: "default values when not set",
			envVars: map[string]string{},
			expectError: false,
			validate: func(t *testing.T, config MetadataConfig) {
				assert.False(t, config.Enabled) // Default is false
				assert.Equal(t, "", config.ClusterName)
				assert.Equal(t, []string{"kube-system", "kube-public", "kube-node-lease"}, config.IgnoredNamespaces)
				assert.Equal(t, "", config.NamespaceLabelSelector)
			},
		},
		{
			name: "cluster name with whitespace - should fail when enabled",
			envVars: map[string]string{
				"METADATA_INJECTION_ENABLED": "true",
				"K8S_CLUSTER_NAME":           "   ",
			},
			expectError: true,
		},
		{
			name: "empty ignored namespaces",
			envVars: map[string]string{
				"METADATA_INJECTION_ENABLED":            "true",
				"K8S_CLUSTER_NAME":                      "test-cluster",
				"METADATA_INJECTION_IGNORED_NAMESPACES": "",
			},
			expectError: false,
			validate: func(t *testing.T, config MetadataConfig) {
				assert.True(t, config.Enabled)
				assert.Equal(t, "test-cluster", config.ClusterName)
				// envconfig parses empty string as empty slice
				assert.Empty(t, config.IgnoredNamespaces)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			clearMetadataEnvVars()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearMetadataEnvVars()

			config, err := LoadConfigFromEnv()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, config)
				}
			}
		})
	}
}

func TestMetadataConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      MetadataConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config - enabled with cluster name",
			config: MetadataConfig{
				Enabled:     true,
				ClusterName: "test-cluster",
			},
			expectError: false,
		},
		{
			name: "valid config - disabled without cluster name",
			config: MetadataConfig{
				Enabled:     false,
				ClusterName: "",
			},
			expectError: false,
		},
		{
			name: "invalid config - enabled without cluster name",
			config: MetadataConfig{
				Enabled:     true,
				ClusterName: "",
			},
			expectError: true,
			errorMsg:    "K8S_CLUSTER_NAME must be set",
		},
		{
			name: "invalid config - enabled with whitespace cluster name",
			config: MetadataConfig{
				Enabled:     true,
				ClusterName: "   ",
			},
			expectError: true,
			errorMsg:    "K8S_CLUSTER_NAME must be set",
		},
		{
			name: "valid config - enabled with whitespace and valid cluster name",
			config: MetadataConfig{
				Enabled:     true,
				ClusterName: "  test-cluster  ",
			},
			expectError: false, // TrimSpace should handle this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMetadataConfig_IsNamespaceIgnored(t *testing.T) {
	tests := []struct {
		name          string
		config        MetadataConfig
		namespace     string
		expectedValue bool
	}{
		{
			name: "namespace in ignore list",
			config: MetadataConfig{
				IgnoredNamespaces: []string{"kube-system", "kube-public"},
			},
			namespace:     "kube-system",
			expectedValue: true,
		},
		{
			name: "namespace not in ignore list",
			config: MetadataConfig{
				IgnoredNamespaces: []string{"kube-system", "kube-public"},
			},
			namespace:     "default",
			expectedValue: false,
		},
		{
			name: "empty ignore list",
			config: MetadataConfig{
				IgnoredNamespaces: []string{},
			},
			namespace:     "kube-system",
			expectedValue: false,
		},
		{
			name: "case sensitive matching",
			config: MetadataConfig{
				IgnoredNamespaces: []string{"kube-system"},
			},
			namespace:     "KUBE-SYSTEM",
			expectedValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsNamespaceIgnored(tt.namespace)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}

// Helper function to clear metadata-related environment variables
func clearMetadataEnvVars() {
	os.Unsetenv("METADATA_INJECTION_ENABLED")
	os.Unsetenv("K8S_CLUSTER_NAME")
	os.Unsetenv("METADATA_INJECTION_IGNORED_NAMESPACES")
	os.Unsetenv("METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR")
}
