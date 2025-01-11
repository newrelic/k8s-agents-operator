package v1alpha2

import (
	"fmt"
	"github.com/newrelic/k8s-agents-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
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

	//dst.Spec.HealthAgent = v1beta1.HealthAgent{} FIXME: Do we need to set anything for the HealthAgent?

	// Status
	// FIXME: Do these have any values we need to set?
	//dst.Status.PodsMatching =
	//dst.Status.PodsInjected =
	//dst.Status.PodsNotReady =
	//dst.Status.PodsOutdated =
	//dst.Status.PodsHealthy =
	//dst.Status.PodsUnhealthy =
	//dst.Status.UnhealthyPodsErrors =
	//dst.Status.LastUpdated =

	return nil
}

// ConvertFrom converts from the Hub version (v1) to this version.
func (dst *Instrumentation) ConvertFrom(srcRaw conversion.Hub) error {
	return fmt.Errorf("downgrade to v1alpha1 is not supported")
}
