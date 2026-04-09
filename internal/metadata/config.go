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
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

// MetadataConfig holds configuration for Kubernetes metadata injection
type MetadataConfig struct {
	// Enabled controls whether metadata injection is active
	Enabled bool `envconfig:"METADATA_INJECTION_ENABLED" default:"false"`

	// ClusterName is the Kubernetes cluster name to inject (required when Enabled=true)
	ClusterName string `envconfig:"K8S_CLUSTER_NAME" default:""`

	// IgnoredNamespaces is a comma-separated list of namespaces to skip
	IgnoredNamespaces []string `envconfig:"METADATA_INJECTION_IGNORED_NAMESPACES" default:"kube-system,kube-public,kube-node-lease"`

	// NamespaceLabelSelector is an optional label selector for namespace filtering
	// Format: "key1=value1,key2=value2"
	// If empty, all namespaces (except ignored ones) will be injected
	// If set, only namespaces matching the selector will be injected
	NamespaceLabelSelector string `envconfig:"METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR" default:""`
}

// LoadConfigFromEnv loads metadata injection configuration from environment variables
func LoadConfigFromEnv() (MetadataConfig, error) {
	var config MetadataConfig
	if err := envconfig.Process("", &config); err != nil {
		return config, fmt.Errorf("failed to load metadata injection config from environment: %w", err)
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return config, fmt.Errorf("invalid metadata injection config: %w", err)
	}

	return config, nil
}

// Validate checks that the configuration is valid
func (c *MetadataConfig) Validate() error {
	// If metadata injection is disabled, no validation needed
	if !c.Enabled {
		return nil
	}

	// Cluster name is required when metadata injection is enabled
	if strings.TrimSpace(c.ClusterName) == "" {
		return fmt.Errorf("K8S_CLUSTER_NAME must be set when METADATA_INJECTION_ENABLED is true")
	}

	return nil
}

// IsNamespaceIgnored checks if a namespace should be ignored based on the ignore list
func (c *MetadataConfig) IsNamespaceIgnored(namespaceName string) bool {
	for _, ignoredNs := range c.IgnoredNamespaces {
		if namespaceName == ignoredNs {
			return true
		}
	}
	return false
}
