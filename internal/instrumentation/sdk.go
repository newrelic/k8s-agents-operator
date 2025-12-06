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
	"fmt"
	"sort"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/apm"
	"github.com/newrelic/k8s-agents-operator/internal/util"
	"github.com/newrelic/k8s-agents-operator/internal/util/svcctx"
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

	containerPorts := assignHealthPorts(ctx, containerInsts, *pod.DeepCopy())
	// loop through all containers which need health, and instrument them.  We need to know which ports to allocate first
	containerVolumes, err := assignHealthVolumes(containerInsts, *pod.DeepCopy())
	if err != nil {
		logger.Error(err, "skipping health agent injection")
	} else {
		logger.V(1).Info("health info", "volumes", containerVolumes, "ports", containerPorts)
		injector := apm.NewHealthInjector()
		for _, containerName := range containerNames {
			for _, inst := range containerInsts[containerName] {
				volume := containerVolumes[containerName]
				port := containerPorts[containerName]
				injector.ConfigureClient(i.client)
				apmCtx := logr.NewContext(ctx, logger.WithValues("injector", injector.Language()))
				logger.V(1).Info("targeting a container for injection",
					"container_name", containerName,
					"instrumentation_namespace", inst.Namespace,
					"instrumentation_name", inst.Name,
					"agent_language", "health",
				)

				mutatedPod, err := injector.Inject(apmCtx, *inst, ns, *pod.DeepCopy(), containerName, port, volume)
				if err != nil {
					logger.Error(err, "skipping agent injection", "agent_language", "health")
					continue
				}
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

const (
	apmHealthLocation = "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION"
	apmHealthPort     = "NEW_RELIC_SIDECAR_LISTEN_PORT"
)

var errPathAlreadyAssigned = fmt.Errorf("path already assigned")

func assignHealthVolumes(candidates map[string][]*current.Instrumentation, pod corev1.Pod) (map[string]string, error) {
	assignedContainerPaths := map[string]string{}
	for containerName := range candidates {
		assignedContainerPaths[containerName] = ""
	}

	for _, container := range pod.Spec.Containers {
		if _, ok := candidates[container.Name]; ok {
			for _, entry := range container.Env {
				if entry.Name == apmHealthLocation {
					path := entry.Value
					if path == "" {
						continue
					}
					assignedContainerPaths[container.Name] = path
					break
				}
			}
		}
	}

	for containerName, candidateInstrumentations := range candidates {
		existingPath := assignedContainerPaths[containerName]
		for _, candidateInstrumentation := range candidateInstrumentations {
			agentsEnv := make([][]corev1.EnvVar, 0, 2)
			if !candidateInstrumentation.Spec.Agent.IsEmpty() {
				agentsEnv = append(agentsEnv, candidateInstrumentation.Spec.Agent.Env)
			}
			if !candidateInstrumentation.Spec.HealthAgent.IsEmpty() {
				agentsEnv = append(agentsEnv, candidateInstrumentation.Spec.HealthAgent.Env)
			}
			for _, agentEnv := range agentsEnv {
				path := ""
				for _, entry := range agentEnv {
					if entry.Name == apmHealthLocation {
						if entry.Value == "" {
							continue
						}
						path = entry.Value
						break
					}
				}
				if path == "" {
					continue
				}
				if existingPath != "" && existingPath != path {
					return nil, errPathAlreadyAssigned
				}
				assignedContainerPaths[containerName] = path
			}
		}
	}

	pathsInUse := map[string]struct{}{}
	for containerName, assignedContainerPath := range assignedContainerPaths {
		if assignedContainerPath == "" {
			assignedContainerPath = "file:///" + apm.GenerateContainerName("nri-health--"+containerName)
			assignedContainerPaths[containerName] = assignedContainerPath
		}
		if _, ok := pathsInUse[assignedContainerPath]; ok {
			return nil, errPathAlreadyAssigned
		}
		pathsInUse[assignedContainerPath] = struct{}{}
	}

	return assignedContainerPaths, nil
}

func assignHealthPorts(ctx context.Context, candidates map[string][]*current.Instrumentation, pod corev1.Pod) map[string]int {
	logger, _ := logr.FromContext(ctx)

	portsInUse := map[int]struct{}{}
	for _, container := range pod.Spec.InitContainers {
		for _, port := range container.Ports {
			portsInUse[int(port.ContainerPort)] = struct{}{}
		}
	}
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			portsInUse[int(port.ContainerPort)] = struct{}{}
		}
	}

	containerNames := make([]string, 0, len(candidates))
	for containerName := range candidates {
		containerNames = append(containerNames, containerName)
	}
	sort.Strings(containerNames)

	assignedPorts := map[string]int{}
	for _, containerName := range containerNames {
		instrumentations := candidates[containerName]
		for _, instrumentation := range instrumentations {
			if instrumentation.Spec.HealthAgent.IsEmpty() {
				continue
			}
			desiredPort := 0 // 0 means dynamic assignment
			for _, entry := range instrumentation.Spec.HealthAgent.Env {
				if entry.Name == apmHealthPort {
					if entry.Value == "" {
						continue
					}
					parsedPort, err := strconv.Atoi(entry.Value)
					if err != nil {
						// this should be an error, but we'll continue, so it still gets a dynamically assigned port.  We'll need the sidecar injection to account for this and override the port based on this by setting the env var as well
						logger.Error(err, "unable to parse value for env var", "key", apmHealthPort, "value", entry.Value)
						parsedPort = 0
					}
					desiredPort = parsedPort
				}
			}
			if _, ok := portsInUse[desiredPort]; ok && desiredPort != 0 {
				// this should be an error, but we'll continue with a dynamically assigned port.  We don't have any way of know if the port will conflict with another container unless it was exposed in the pod spec
				desiredPort = 0
			}
			if desiredPort > 0 {
				portsInUse[desiredPort] = struct{}{}
			}
			assignedPorts[containerName] = desiredPort
		}
	}

	for _, containerName := range containerNames {
		if port := assignedPorts[containerName]; port == 0 {
			// assume 59k is enough ports to check.  we have more problems if those are all in use
			for i := 6194; i < 65536; i++ {
				if _, ok := portsInUse[i]; !ok {
					portsInUse[i] = struct{}{}
					assignedPorts[containerName] = i
					break
				}
			}
		}
	}
	return assignedPorts
}
