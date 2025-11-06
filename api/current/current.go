package current

import currentapi "github.com/newrelic/k8s-agents-operator/api/v1beta3"

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

	ContainerSelector = currentapi.ContainerSelector
	EnvSelector       = currentapi.EnvSelector
	ImageSelector     = currentapi.ImageSelector
	NameSelector      = currentapi.NameSelector

	EnvSelectorOperator   = currentapi.EnvSelectorOperator
	ImageSelectorOperator = currentapi.ImageSelectorOperator
	NameSelectorOperator  = currentapi.NameSelectorOperator

	ImageSelectorKey = currentapi.ImageSelectorKey
	NameSelectorKey  = currentapi.NameSelectorKey

	EnvSelectorRequirement   = currentapi.EnvSelectorRequirement
	ImageSelectorRequirement = currentapi.ImageSelectorRequirement
	NameSelectorRequirement  = currentapi.NameSelectorRequirement
)

var (
	AddToScheme   = currentapi.AddToScheme
	GroupVersion  = currentapi.GroupVersion
	SchemeBuilder = currentapi.SchemeBuilder
)
