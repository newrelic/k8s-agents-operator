package current

import currentapi "github.com/newrelic/k8s-agents-operator/api/v1beta2"

type (
	Agent                    = currentapi.Agent
	HealthAgent              = currentapi.HealthAgent
	Instrumentation          = currentapi.Instrumentation
	InstrumentationDefaulter = currentapi.InstrumentationDefaulter
	InstrumentationList      = currentapi.InstrumentationList
	InstrumentationSpec      = currentapi.InstrumentationSpec
	InstrumentationStatus    = currentapi.InstrumentationStatus
	InstrumentationValidator = currentapi.InstrumentationValidator
	UnhealthyPodError        = currentapi.UnhealthyPodError
)

var (
	AddToScheme   = currentapi.AddToScheme
	GroupVersion  = currentapi.GroupVersion
	SchemeBuilder = currentapi.SchemeBuilder
)
