/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package instrumentation

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/newrelic/k8s-agents-operator/internal/util/svcctx"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/apm"
	"github.com/newrelic/k8s-agents-operator/internal/util"
	"github.com/newrelic/k8s-agents-operator/internal/version"
)

const (
	DefaultLicenseKeySecretName            = "newrelic-key-secret"
	DescK8sAgentOperatorVersionLabelName   = "newrelic-k8s-agents-operator-version"
	DescK8sAgentOperatorTXIDAnnotationName = "newrelic-k8s-agents-operator-txid"
)

// compile time type assertion
var _ SdkContainerInjector = (*NewrelicSdkInjector)(nil)

type SdkContainerInjector interface {
	InjectContainers(ctx context.Context, containerInsts map[string][]*current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod
}

// NewrelicSdkInjector is the base struct used to inject our instrumentation into a pod
type NewrelicSdkInjector struct {
	client           client.Client
	injectorRegistry *apm.InjectorRegistery
}

// NewNewrelicSdkInjector is used to create our injector
func NewNewrelicSdkInjector(client client.Client, injectorRegistry *apm.InjectorRegistery) *NewrelicSdkInjector {
	return &NewrelicSdkInjector{
		client:           client,
		injectorRegistry: injectorRegistry,
	}
}

func (i *NewrelicSdkInjector) InjectContainers(ctx context.Context, containerInsts map[string][]*current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod {
	if len(pod.Spec.Containers) == 0 {
		return pod
	}
	logger, _ := logr.FromContext(ctx)

	containerNames := make([]string, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	for _, container := range pod.Spec.InitContainers {
		if _, ok := containerInsts[container.Name]; ok {
			containerNames = append(containerNames, container.Name)
		}
	}
	for _, container := range pod.Spec.Containers {
		if _, ok := containerInsts[container.Name]; ok {
			containerNames = append(containerNames, container.Name)
		}
	}

	hadMatchingInjector := false
	successfulInjection := false
	for _, containerName := range containerNames {
		for _, inst := range containerInsts[containerName] {
			for _, injector := range i.injectorRegistry.GetInjectors() {
				if !injector.Accepts(*inst, ns, pod) {
					continue
				}
				hadMatchingInjector = true
				injector.ConfigureClient(i.client)
				apmCtx := logr.NewContext(ctx, logger.WithValues("injector", injector.Language()))
				logger.V(1).Info("targeting a container for injection",
					"container_name", containerName,
					"instrumentation_namespace", inst.Namespace,
					"instrumentation_name", inst.Name,
					"agent_language", inst.Spec.Agent.Language,
				)
				mutatedPod, err := injector.InjectContainer(apmCtx, *inst, ns, *pod.DeepCopy(), containerName)
				if err != nil {
					logger.Error(err, "skipping agent injection", "agent_language", inst.Spec.Agent.Language)
					continue
				}
				successfulInjection = true
				pod = mutatedPod
			}
		}
	}
	if !hadMatchingInjector {
		logger.Info("no language agents found while trying to instrument pod",
			"pod_details", pod.String(),
			"registered_injectors", i.injectorRegistry.GetInjectors().Names(),
		)
	}
	if successfulInjection {
		util.SetPodLabel(&pod, DescK8sAgentOperatorVersionLabelName, version.Get().Operator)
		if txid, ok := svcctx.TXIDFromContext(ctx); ok {
			util.SetPodAnnotation(&pod, DescK8sAgentOperatorTXIDAnnotationName, txid)
		}
	}
	return pod
}
