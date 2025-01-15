package v1alpha2

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/newrelic/k8s-agents-operator/api/v1beta1"
)

// ConvertTo converts this Instrumentation to the Hub version (v1beta1).
func (src *Instrumentation) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.Instrumentation)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.Exporter = v1beta1.Exporter{
		Endpoint: src.Spec.Exporter.Endpoint,
	}
	dst.Spec.Resource = v1beta1.Resource{
		Attributes:          src.Spec.Resource.Attributes,
		AddK8sUIDAttributes: src.Spec.Resource.AddK8sUIDAttributes,
	}
	dst.Spec.Propagators = src.Spec.Propagators
	dst.Spec.Sampler = v1beta1.Sampler{
		Type:     src.Spec.Sampler.Type,
		Argument: src.Spec.Sampler.Argument,
	}
	dst.Spec.PodLabelSelector = src.Spec.PodLabelSelector
	dst.Spec.NamespaceLabelSelector = src.Spec.NamespaceLabelSelector
	dst.Spec.LicenseKeySecret = src.Spec.LicenseKeySecret
	dst.Spec.AgentConfigMap = ""
	dst.Spec.Agent = v1beta1.Agent{
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
	src := srcRaw.(*v1beta1.Instrumentation)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.Exporter = Exporter{
		Endpoint: src.Spec.Exporter.Endpoint,
	}
	dst.Spec.Resource = Resource{
		Attributes:          src.Spec.Resource.Attributes,
		AddK8sUIDAttributes: src.Spec.Resource.AddK8sUIDAttributes,
	}
	dst.Spec.Propagators = src.Spec.Propagators
	dst.Spec.Sampler = Sampler{
		Type:     src.Spec.Sampler.Type,
		Argument: src.Spec.Sampler.Argument,
	}
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
