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
	AddToScheme             = currentapi.AddToScheme
	GroupVersion            = currentapi.GroupVersion
	SchemeBuilder           = currentapi.SchemeBuilder
	SetupWebhookWithManager = currentapi.SetupWebhookWithManager

	EnvSelectorOpIn           = currentapi.EnvSelectorOpIn
	EnvSelectorOpNotIn        = currentapi.EnvSelectorOpNotIn
	EnvSelectorOpEquals       = currentapi.EnvSelectorOpEquals
	EnvSelectorOpNotEquals    = currentapi.EnvSelectorOpNotEquals
	EnvSelectorOpExists       = currentapi.EnvSelectorOpExists
	EnvSelectorOpDoesNotExist = currentapi.EnvSelectorOpDoesNotExist

	NameSelectorKeyInitContainer = currentapi.NameSelectorKeyInitContainer
	NameSelectorKeyContainer     = currentapi.NameSelectorKeyContainer
	NameSelectorKeyAnyContainer  = currentapi.NameSelectorKeyAnyContainer

	NameSelectorOpEquals    = currentapi.NameSelectorOpEquals
	NameSelectorOpNotEquals = currentapi.NameSelectorOpNotEquals
	NameSelectorOpIn        = currentapi.NameSelectorOpIn
	NameSelectorOpNotIn     = currentapi.NameSelectorOpNotIn

	ImageSelectorKeyUrl = currentapi.ImageSelectorKeyUrl

	ImageSelectorOpEquals        = currentapi.ImageSelectorOpEquals
	ImageSelectorOpNotEquals     = currentapi.ImageSelectorOpNotEquals
	ImageSelectorOpIn            = currentapi.ImageSelectorOpIn
	ImageSelectorOpNotIn         = currentapi.ImageSelectorOpNotIn
	ImageSelectorOpStartsWith    = currentapi.ImageSelectorOpStartsWith
	ImageSelectorOpNotStartsWith = currentapi.ImageSelectorOpNotStartsWith
	ImageSelectorOpEndsWith      = currentapi.ImageSelectorOpEndsWith
	ImageSelectorOpNotEndsWith   = currentapi.ImageSelectorOpNotEndsWith
	ImageSelectorOpContains      = currentapi.ImageSelectorOpContains
	ImageSelectorOpNotContains   = currentapi.ImageSelectorOpNotContains
)
