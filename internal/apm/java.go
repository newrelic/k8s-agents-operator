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
package apm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/util"
)

const (
	envJavaToolsOptions   = "JAVA_TOOL_OPTIONS"
	envApmConfigFile      = "NEWRELIC_FILE"
	javaJVMArgument       = "-javaagent:/newrelic-instrumentation/newrelic-agent.jar"
	javaInitContainerName = initContainerName + "-java"
	javaApmConfigPath     = apmConfigMountPath + "/newrelic.yaml"
)

var _ Injector = (*JavaInjector)(nil)
var _ ContainerInjector = (*JavaInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&JavaInjector{baseInjector{lang: "java"}})
}

type JavaInjector struct {
	baseInjector
}

func (i *JavaInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	return i.InjectContainer(ctx, inst, ns, pod, pod.Spec.Containers[0].Name)
}

func (i *JavaInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}

	if err := validateContainerEnv(container.Env, envJavaToolsOptions); err != nil {
		return corev1.Pod{}, err
	}
	setEnvVar(container, envJavaToolsOptions, javaJVMArgument, true, " ")
	setContainerEnvFromInst(container, inst)

	if inst.Spec.AgentConfigMap != "" {
		setAgentConfigMap(&pod, inst.Spec.AgentConfigMap, container)

		// Add ENV
		if apmIdx := getIndexOfEnv(container.Env, envApmConfigFile); apmIdx == -1 {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  envApmConfigFile,
				Value: javaApmConfigPath,
			})
		}
	}

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/newrelic-instrumentation",
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, javaInitContainerName) {
		if isPodVolumeMissing(pod, volumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		newContainer := corev1.Container{
			Name:    javaInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"cp", "/newrelic-agent.jar", "/newrelic-instrumentation/newrelic-agent.jar"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
		}
		if isTargetInitContainer {
			index := getInitContainerIndex(pod, containerName)
			pod.Spec.InitContainers = insertContainerBeforeIndex(pod.Spec.InitContainers, index, newContainer)
		} else {
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, newContainer)
		}
	} else {
		if isTargetInitContainer {
			agentIndex := getInitContainerIndex(pod, rubyInitContainerName)
			targetIndex := getInitContainerIndex(pod, containerName)
			if targetIndex < agentIndex {
				// move our agent before the target, so that it runs before the target!
				var agentContainer corev1.Container
				pod.Spec.InitContainers, agentContainer = removeContainerByIndex(pod.Spec.InitContainers, agentIndex)
				pod.Spec.InitContainers = insertContainerBeforeIndex(pod.Spec.InitContainers, targetIndex, agentContainer)
			}
		}
	}

	if err := i.setContainerEnvAppName(ctx, &ns, &pod, container); err != nil {
		return corev1.Pod{}, err
	}
	setContainerEnvInjectionDefaults(container)
	setContainerEnvLicenseKey(container, inst.Spec.LicenseKeySecret)
	if err := setPodAnnotationFromInstrumentationVersion(&pod, inst); err != nil {
		return corev1.Pod{}, err
	}
	return i.injectHealthWithContainer(ctx, inst, ns, pod, container)
}
