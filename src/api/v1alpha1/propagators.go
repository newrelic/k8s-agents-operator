package v1alpha1

type (
	// Propagator represents the propagation type.
	// +kubebuilder:validation:Enum=tracecontext;none
	Propagator string
)

const (
	// TraceContext represents W3C Trace Context.
	TraceContext Propagator = "tracecontext"
	// None represents automatically configured propagator.
	None Propagator = "none"
)
