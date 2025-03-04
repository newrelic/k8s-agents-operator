package current

import "github.com/newrelic/k8s-agents-operator/api/v1beta1"

type (
	Agent                    = v1beta1.Agent
	HealthAgent              = v1beta1.HealthAgent
	Instrumentation          = v1beta1.Instrumentation
	InstrumentationDefaulter = v1beta1.InstrumentationDefaulter
	InstrumentationList      = v1beta1.InstrumentationList
	InstrumentationSpec      = v1beta1.InstrumentationSpec
	InstrumentationStatus    = v1beta1.InstrumentationStatus
	InstrumentationValidator = v1beta1.InstrumentationValidator
	UnhealthyPodError        = v1beta1.UnhealthyPodError
)

var (
	AddToScheme   = v1beta1.AddToScheme
	GroupVersion  = v1beta1.GroupVersion
	SchemeBuilder = v1beta1.SchemeBuilder
)
