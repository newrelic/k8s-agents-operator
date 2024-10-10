package v1alpha2

type (
	// SamplerType represents sampler type.
	// +kubebuilder:validation:Enum=always_on;always_off;traceidratio;parentbased_always_on;parentbased_always_off;parentbased_traceidratio
	SamplerType string
)

const (
	// AlwaysOn represents AlwaysOnSampler.
	AlwaysOn SamplerType = "always_on"
	// AlwaysOff represents AlwaysOffSampler.
	AlwaysOff SamplerType = "always_off"
	// TraceIDRatio represents TraceIdRatioBased.
	TraceIDRatio SamplerType = "traceidratio"
	// ParentBasedAlwaysOn represents ParentBased(root=AlwaysOnSampler).
	ParentBasedAlwaysOn SamplerType = "parentbased_always_on"
	// ParentBasedAlwaysOff represents ParentBased(root=AlwaysOffSampler).
	ParentBasedAlwaysOff SamplerType = "parentbased_always_off"
	// ParentBasedTraceIDRatio represents ParentBased(root=TraceIdRatioBased).
	ParentBasedTraceIDRatio SamplerType = "parentbased_traceidratio"
)
