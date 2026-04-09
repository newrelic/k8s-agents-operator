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

// Package metadata provides Kubernetes metadata injection for pod containers.
//
// This package implements a webhook mutator that injects Kubernetes metadata
// as environment variables into pod containers, enabling APM agents to correlate
// application traces with infrastructure monitoring.
//
// The metadata injection mutator adds the following environment variables:
//   - NEW_RELIC_METADATA_KUBERNETES_CLUSTER_NAME: Cluster name (from config)
//   - NEW_RELIC_METADATA_KUBERNETES_NODE_NAME: Node name (downward API)
//   - NEW_RELIC_METADATA_KUBERNETES_NAMESPACE_NAME: Namespace name (downward API)
//   - NEW_RELIC_METADATA_KUBERNETES_POD_NAME: Pod name (downward API)
//   - NEW_RELIC_METADATA_KUBERNETES_DEPLOYMENT_NAME: Deployment name (derived from pod metadata)
//   - NEW_RELIC_METADATA_KUBERNETES_CONTAINER_NAME: Container name (static)
//   - NEW_RELIC_METADATA_KUBERNETES_CONTAINER_IMAGE_NAME: Container image (static)
//
// Configuration is loaded from environment variables:
//   - METADATA_INJECTION_ENABLED: Enable/disable metadata injection (default: false)
//   - K8S_CLUSTER_NAME: Kubernetes cluster name (required when enabled)
//   - METADATA_INJECTION_IGNORED_NAMESPACES: Comma-separated list of namespaces to skip
//   - METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR: Optional label selector for namespace filtering
//
// Namespace Filtering:
//
// Metadata injection can be filtered by:
//  1. Ignore list: System namespaces (kube-system, kube-public, kube-node-lease) are skipped by default
//  2. Opt-out annotation: Namespaces with annotation "newrelic.com/metadata-injection: false" are skipped
//  3. Label selector: If configured, only namespaces matching the label selector receive metadata injection
//
// The mutator is designed to work independently of agent injection and can be
// enabled/disabled without affecting APM agent instrumentation.
package metadata
