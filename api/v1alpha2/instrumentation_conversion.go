package v1alpha2

import (
	"github.com/newrelic/k8s-agents-operator/api/current"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this Instrumentation to the Hub version (v1beta1).
func (src *Instrumentation) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*current.Instrumentation)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.PodLabelSelector = src.Spec.PodLabelSelector
	dst.Spec.NamespaceLabelSelector = src.Spec.NamespaceLabelSelector
	dst.Spec.LicenseKeySecret = src.Spec.LicenseKeySecret
	dst.Spec.AgentConfigMap = ""
	dst.Spec.Agent = current.Agent{
		Language:        src.Spec.Agent.Language,
		Image:           src.Spec.Agent.Image,
		VolumeSizeLimit: src.Spec.Agent.VolumeSizeLimit,
		Env:             src.Spec.Agent.Env,
		Resources:       src.Spec.Agent.Resources,
	}

	// HealthAgent and Status are empty

	return nil
}

// ConvertFrom converts from the Hub version (v1) to this version.
func (dst *Instrumentation) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*current.Instrumentation)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.PodLabelSelector = src.Spec.PodLabelSelector
	dst.Spec.NamespaceLabelSelector = src.Spec.NamespaceLabelSelector
	dst.Spec.LicenseKeySecret = src.Spec.LicenseKeySecret
	dst.Spec.Agent = Agent{
		Language:        src.Spec.Agent.Language,
		Image:           src.Spec.Agent.Image,
		VolumeSizeLimit: src.Spec.Agent.VolumeSizeLimit,
		Env:             src.Spec.Agent.Env,
		Resources:       src.Spec.Agent.Resources,
	}

	return nil
}
