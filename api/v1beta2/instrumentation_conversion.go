package v1beta2

import (
	"github.com/newrelic/k8s-agents-operator/api/current"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this Instrumentation to the Hub version.
func (src *Instrumentation) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*current.Instrumentation)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	for _, srcExpr := range src.Spec.ContainerSelector.ImageSelector.MatchExpressions {
		dst.Spec.ContainerSelector.ImageSelector.MatchExpressions = append(dst.Spec.ContainerSelector.ImageSelector.MatchExpressions, current.ImageSelectorRequirement{
			Key: current.ImageSelectorKey(srcExpr.Key), Operator: current.ImageSelectorOperator(srcExpr.Operator), Values: srcExpr.Values,
		})
	}
	for _, srcExpr := range src.Spec.ContainerSelector.EnvSelector.MatchExpressions {
		dst.Spec.ContainerSelector.EnvSelector.MatchExpressions = append(dst.Spec.ContainerSelector.EnvSelector.MatchExpressions, current.EnvSelectorRequirement{
			Key: srcExpr.Key, Operator: current.EnvSelectorOperator(srcExpr.Operator), Values: srcExpr.Values,
		})
	}
	for _, srcExpr := range src.Spec.ContainerSelector.NameSelector.MatchExpressions {
		dst.Spec.ContainerSelector.NameSelector.MatchExpressions = append(dst.Spec.ContainerSelector.NameSelector.MatchExpressions, current.NameSelectorRequirement{
			Key: current.NameSelectorKey(srcExpr.Key), Operator: current.NameSelectorOperator(srcExpr.Operator), Values: srcExpr.Values,
		})
	}

	dst.Spec.ContainerSelector.ImageSelector.MatchImages = src.Spec.ContainerSelector.ImageSelector.MatchImages
	dst.Spec.ContainerSelector.EnvSelector.MatchEnvs = src.Spec.ContainerSelector.EnvSelector.MatchEnvs
	dst.Spec.ContainerSelector.NameSelector.MatchNames = src.Spec.ContainerSelector.NameSelector.MatchNames
	dst.Spec.ContainerSelector.NamesFromPodAnnotation = src.Spec.ContainerSelector.NamesFromPodAnnotation

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
		ImagePullPolicy: src.Spec.Agent.ImagePullPolicy,
		SecurityContext: src.Spec.Agent.SecurityContext,
	}
	dst.Spec.HealthAgent = current.HealthAgent{
		Image:           src.Spec.HealthAgent.Image,
		Env:             src.Spec.HealthAgent.Env,
		ImagePullPolicy: src.Spec.HealthAgent.ImagePullPolicy,
		SecurityContext: src.Spec.HealthAgent.SecurityContext,
		Resources:       src.Spec.HealthAgent.Resources,
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
		ObservedVersion:     src.Status.ObservedVersion,
	}
	return nil
}

// ConvertFrom converts from the Hub version to this version.
func (dst *Instrumentation) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*current.Instrumentation)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec

	for _, srcExpr := range src.Spec.ContainerSelector.ImageSelector.MatchExpressions {
		dst.Spec.ContainerSelector.ImageSelector.MatchExpressions = append(dst.Spec.ContainerSelector.ImageSelector.MatchExpressions, ImageSelectorRequirement{
			Key: ImageSelectorKey(srcExpr.Key), Operator: ImageSelectorOperator(srcExpr.Operator), Values: srcExpr.Values,
		})
	}
	for _, srcExpr := range src.Spec.ContainerSelector.EnvSelector.MatchExpressions {
		dst.Spec.ContainerSelector.EnvSelector.MatchExpressions = append(dst.Spec.ContainerSelector.EnvSelector.MatchExpressions, EnvSelectorRequirement{
			Key: srcExpr.Key, Operator: EnvSelectorOperator(srcExpr.Operator), Values: srcExpr.Values,
		})
	}
	for _, srcExpr := range src.Spec.ContainerSelector.NameSelector.MatchExpressions {
		dst.Spec.ContainerSelector.NameSelector.MatchExpressions = append(dst.Spec.ContainerSelector.NameSelector.MatchExpressions, NameSelectorRequirement{
			Key: NameSelectorKey(srcExpr.Key), Operator: NameSelectorOperator(srcExpr.Operator), Values: srcExpr.Values,
		})
	}

	dst.Spec.ContainerSelector.ImageSelector.MatchImages = src.Spec.ContainerSelector.ImageSelector.MatchImages
	dst.Spec.ContainerSelector.EnvSelector.MatchEnvs = src.Spec.ContainerSelector.EnvSelector.MatchEnvs
	dst.Spec.ContainerSelector.NameSelector.MatchNames = src.Spec.ContainerSelector.NameSelector.MatchNames
	dst.Spec.ContainerSelector.NamesFromPodAnnotation = src.Spec.ContainerSelector.NamesFromPodAnnotation

	dst.Spec.PodLabelSelector = src.Spec.PodLabelSelector
	dst.Spec.NamespaceLabelSelector = src.Spec.NamespaceLabelSelector
	dst.Spec.LicenseKeySecret = src.Spec.LicenseKeySecret
	dst.Spec.Agent = Agent{
		Env:             src.Spec.Agent.Env,
		Image:           src.Spec.Agent.Image,
		ImagePullPolicy: src.Spec.Agent.ImagePullPolicy,
		Language:        src.Spec.Agent.Language,
		Resources:       src.Spec.Agent.Resources,
		SecurityContext: src.Spec.Agent.SecurityContext,
		VolumeSizeLimit: src.Spec.Agent.VolumeSizeLimit,
	}
	dst.Spec.HealthAgent = HealthAgent{
		Env:             src.Spec.HealthAgent.Env,
		Image:           src.Spec.HealthAgent.Image,
		ImagePullPolicy: src.Spec.HealthAgent.ImagePullPolicy,
		Resources:       src.Spec.HealthAgent.Resources,
		SecurityContext: src.Spec.HealthAgent.SecurityContext,
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
		ObservedVersion:     src.Status.ObservedVersion,
	}
	return nil
}
