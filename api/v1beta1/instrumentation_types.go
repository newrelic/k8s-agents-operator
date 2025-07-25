/*
Copyright 2025.

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

package v1beta1

import (
	"github.com/newrelic/k8s-agents-operator/api/common"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstrumentationSpec defines the desired state of Instrumentation
type InstrumentationSpec struct {
	// Exporter defines exporter configuration.
	// +optional
	Exporter `json:"exporter,omitempty"`

	// Resource defines the configuration for the resource attributes, as defined by the OpenTelemetry specification.
	// +optional
	Resource Resource `json:"resource,omitempty"`

	// Propagators defines inter-process context propagation configuration.
	// Values in this list will be set in the OTEL_PROPAGATORS env var.
	// Enum=tracecontext;none
	// +optional
	Propagators []common.Propagator `json:"propagators,omitempty"`

	// Sampler defines sampling configuration.
	// +optional
	Sampler `json:"sampler,omitempty"`

	// PodLabelSelector defines to which pods the config should be applied.
	// +optional
	PodLabelSelector metav1.LabelSelector `json:"podLabelSelector"`

	// PodLabelSelector defines to which pods the config should be applied.
	// +optional
	NamespaceLabelSelector metav1.LabelSelector `json:"namespaceLabelSelector"`

	// LicenseKeySecret defines where to take the licenseKeySecret from.
	// it should be present in the operator namespace.
	// +optional
	LicenseKeySecret string `json:"licenseKeySecret,omitempty"`

	// AgentConfigMap defines where to take the agent configuration from.
	// it should be present in the operator namespace.
	// +optional
	AgentConfigMap string `json:"agentConfigMap,omitempty"`

	// Agent defines configuration for agent instrumentation.
	Agent Agent `json:"agent,omitempty"`

	// HealthAgent defines configuration for healthAgent instrumentation.
	HealthAgent HealthAgent `json:"healthAgent,omitempty"`
}

// Resource is the attributes that are added to the resource
type Resource struct {
	// Attributes defines attributes that are added to the resource.
	// For example environment: dev
	// +optional
	Attributes map[string]string `json:"resourceAttributes,omitempty"`

	// AddK8sUIDAttributes defines whether K8s UID attributes should be collected (e.g. k8s.deployment.uid).
	// +optional
	AddK8sUIDAttributes bool `json:"addK8sUIDAttributes,omitempty"`
}

// Exporter defines OTLP exporter configuration.
type Exporter struct {
	// Endpoint is address of the collector with OTLP endpoint.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// Sampler defines sampling configuration.
type Sampler struct {
	// Type defines sampler type.
	// The value will be set in the OTEL_TRACES_SAMPLER env var.
	// The value can be for instance parentbased_always_on, parentbased_always_off, parentbased_traceidratio...
	// +optional
	Type common.SamplerType `json:"type,omitempty"`

	// Argument defines sampler argument.
	// The value depends on the sampler type.
	// For instance for parentbased_traceidratio sampler type it is a number in range [0..1] e.g. 0.25.
	// The value will be set in the OTEL_TRACES_SAMPLER_ARG env var.
	// +optional
	Argument string `json:"argument,omitempty"`
}

// Agent is the configuration for the agent
type Agent struct {
	// Language is the language that will be instrumented.
	Language string `json:"language,omitempty"`

	// Image is a container image with Go SDK and auto-instrumentation.
	Image string `json:"image,omitempty"`

	// VolumeSizeLimit defines size limit for volume used for auto-instrumentation.
	// The default size is 200Mi.
	// +optional
	VolumeSizeLimit *resource.Quantity `json:"volumeLimitSize,omitempty"`

	// Env defines Go specific env vars. There are four layers for env vars' definitions and
	// the precedence order is: `original container env vars` > `language specific env vars` > `common env vars` > `instrument spec configs' vars`.
	// If the former var had been defined, then the other vars would be ignored.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources describes the compute resource requirements.
	// +optional
	Resources corev1.ResourceRequirements `json:"resourceRequirements,omitempty"`
}

// IsEmpty is used to check if the agent is empty, excluding `.Language`
func (a *Agent) IsEmpty() bool {
	return a.Image == "" &&
		len(a.Env) == 0 &&
		a.VolumeSizeLimit == nil &&
		len(a.Resources.Limits) == 0 &&
		len(a.Resources.Requests) == 0 &&
		len(a.Resources.Claims) == 0
}

// HealthAgent is the configuration for the healthAgent
type HealthAgent struct {
	// Image is a container image with Go SDK and auto-instrumentation.
	Image string `json:"image,omitempty"`

	// Env defines Go specific env vars. There are four layers for env vars' definitions and
	// the precedence order is: `original container env vars` > `language specific env vars` > `common env vars` > `instrument spec configs' vars`.
	// If the former var had been defined, then the other vars would be ignored.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

type UnhealthyPodError struct {
	Pod       string `json:"pod,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// InstrumentationStatus defines the observed state of Instrumentation
type InstrumentationStatus struct {
	PodsMatching        int64               `json:"podsMatching,omitempty"`
	PodsInjected        int64               `json:"podsInjected,omitempty"`
	PodsNotReady        int64               `json:"podsNotReady,omitempty"`
	PodsOutdated        int64               `json:"podsOutdated,omitempty"`
	PodsHealthy         int64               `json:"podsHealthy,omitempty"`
	PodsUnhealthy       int64               `json:"podsUnhealthy,omitempty"`
	UnhealthyPodsErrors []UnhealthyPodError `json:"unhealthyPodsErrors,omitempty"`
	LastUpdated         metav1.Time         `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=nragent;nragents
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="PodsMatching",type="integer",JSONPath=".status.podsMatching"
// +kubebuilder:printcolumn:name="PodsInjected",type="integer",JSONPath=".status.podsInjected"
// +kubebuilder:printcolumn:name="PodsNotReady",type="integer",JSONPath=".status.podsNotReady"
// +kubebuilder:printcolumn:name="PodsOutdated",type="integer",JSONPath=".status.podsOutdated"
// +kubebuilder:printcolumn:name="PodsHealthy",type="integer",JSONPath=".status.podsHealthy"
// +kubebuilder:printcolumn:name="PodsUnhealthy",type="integer",JSONPath=".status.podsUnhealthy"
// +operator-sdk:csv:customresourcedefinitions:displayName="New Relic Instrumentation"
// +operator-sdk:csv:customresourcedefinitions:resources={{Pod,v1}}
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:object:resource:scope=Namespaced

// Instrumentation is the Schema for the instrumentations API
type Instrumentation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstrumentationSpec   `json:"spec,omitempty"`
	Status InstrumentationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InstrumentationList contains a list of Instrumentation
type InstrumentationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Instrumentation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Instrumentation{}, &InstrumentationList{})
}
