package v1beta1

import (
	"github.com/newrelic/k8s-agents-operator/api/current"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this Instrumentation to the Hub version (v1beta1).
func (src *Instrumentation) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*current.Instrumentation)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.PodLabelSelector = src.Spec.PodLabelSelector
	dst.Spec.NamespaceLabelSelector = src.Spec.NamespaceLabelSelector
	dst.Spec.LicenseKeySecret = src.Spec.LicenseKeySecret
	dst.Spec.AgentConfigMap = src.Spec.AgentConfigMap
	dst.Spec.Agent = current.Agent{
		Language:        src.Spec.Agent.Language,
		Image:           src.Spec.Agent.Image,
		VolumeSizeLimit: src.Spec.Agent.VolumeSizeLimit,
		Env:             src.Spec.Agent.Env,
		Resources:       src.Spec.Agent.Resources,
	}
	dst.Spec.HealthAgent = current.HealthAgent{
		Image: src.Spec.HealthAgent.Image,
		Env:   src.Spec.HealthAgent.Env,
	}
	var unhealthyPodErrors []current.UnhealthyPodError
	if l := len(src.Status.UnhealthyPodsErrors); l > 0 {
		unhealthyPodErrors = make([]current.UnhealthyPodError, l)
		for i, e := range src.Status.UnhealthyPodsErrors {
			unhealthyPodErrors[i] = current.UnhealthyPodError{
				Pod:       e.Pod,
				LastError: e.LastError,
			}
		}
	}
	dst.Status = current.InstrumentationStatus{
		PodsMatching:        src.Status.PodsMatching,
		PodsUnhealthy:       src.Status.PodsUnhealthy,
		PodsHealthy:         src.Status.PodsHealthy,
		PodsInjected:        src.Status.PodsInjected,
		PodsOutdated:        src.Status.PodsOutdated,
		PodsNotReady:        src.Status.PodsNotReady,
		UnhealthyPodsErrors: unhealthyPodErrors,
		LastUpdated:         src.Status.LastUpdated,
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
	dst.Spec.HealthAgent = HealthAgent{
		Image: src.Spec.HealthAgent.Image,
		Env:   src.Spec.HealthAgent.Env,
	}
	dst.Spec.AgentConfigMap = src.Spec.AgentConfigMap
	var unhealthyPodErrors []UnhealthyPodError
	if l := len(src.Status.UnhealthyPodsErrors); l > 0 {
		unhealthyPodErrors = make([]UnhealthyPodError, l)
		for i, e := range src.Status.UnhealthyPodsErrors {
			unhealthyPodErrors[i] = UnhealthyPodError{
				Pod:       e.Pod,
				LastError: e.LastError,
			}
		}
	}
	dst.Status = InstrumentationStatus{
		PodsMatching:        src.Status.PodsMatching,
		PodsUnhealthy:       src.Status.PodsUnhealthy,
		PodsHealthy:         src.Status.PodsHealthy,
		PodsInjected:        src.Status.PodsInjected,
		PodsOutdated:        src.Status.PodsOutdated,
		PodsNotReady:        src.Status.PodsNotReady,
		UnhealthyPodsErrors: unhealthyPodErrors,
		LastUpdated:         src.Status.LastUpdated,
	}

	return nil
}
