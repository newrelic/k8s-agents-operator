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
	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

const (
	envJavaToolsOptions   = "JAVA_TOOL_OPTIONS"
	envApmConfigFile      = "NEWRELIC_FILE"
	javaJVMArgument       = "-javaagent:/newrelic-instrumentation/newrelic-agent.jar"
	javaInitContainerName = initContainerName + "-java"
	javaApmConfigPath     = apmConfigMountPath + "/newrelic.yaml"
)

var _ Injector = (*JavaInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&JavaInjector{baseInjector{lang: "java"}})
}

type JavaInjector struct {
	baseInjector
}

func (i *JavaInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	firstContainer := 0
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	if err := validateContainerEnv(container.Env, envJavaToolsOptions); err != nil {
		return corev1.Pod{}, err
	}
	setEnvVar(container, envJavaToolsOptions, javaJVMArgument, true, " ")
	setContainerEnvFromInst(container, inst)

	if inst.Spec.AgentConfigMap != "" {
		injectAgentConfigMap(&pod, firstContainer, inst.Spec.AgentConfigMap)

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

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:    javaInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"cp", "/newrelic-agent.jar", "/newrelic-instrumentation/newrelic-agent.jar"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
			Resources: *inst.Spec.Agent.Resources.DeepCopy(),
		})
	}

	pod = i.injectNewrelicConfig(ctx, ns, pod, firstContainer, inst.Spec.LicenseKeySecret)

	pod = addAnnotationToPodFromInstrumentationVersion(ctx, pod, inst)

	var err error
	if pod, err = i.injectHealth(ctx, inst, ns, pod, firstContainer, -1); err != nil {
		return corev1.Pod{}, err
	}

	return pod, nil
}
