package v1alpha1

type (
	// SamplerType represents sampler type.
	// +kubebuilder:validation:Enum=always_on;always_off;traceidratio;parentbased_always_on;parentbased_always_off;parentbased_traceidratio
	SamplerType string
)

const (
	// ParentBasedTraceIDRatio represents ParentBased(root=TraceIdRatioBased).
	ParentBasedTraceIDRatio SamplerType = "parentbased_traceidratio"
)
