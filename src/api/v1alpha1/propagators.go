package v1alpha1

type (
	// Propagator represents the propagation type.
	// +kubebuilder:validation:Enum=tracecontext;none
	Propagator string
)
