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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/util"
)

const (
	envDotnetCoreClrEnableProfiling     = "CORECLR_ENABLE_PROFILING"
	envDotnetCoreClrProfiler            = "CORECLR_PROFILER"
	envDotnetCoreClrProfilerPath        = "CORECLR_PROFILER_PATH"
	envDotnetNewrelicHome               = "CORECLR_NEWRELIC_HOME"
	dotnetCoreClrEnableProfilingEnabled = "1"
	dotnetCoreClrProfilerID             = "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"
	dotnetCoreClrProfilerPath           = "/newrelic-instrumentation/libNewRelicProfiler.so"
	dotnetNewrelicHomePath              = "/newrelic-instrumentation"
	dotnetInitContainerName             = initContainerName + "-dotnet"
)

var errUnableToConfigureEnv = errors.New("unable to configure environment variables, they've already been set to different values")

var _ Injector = (*DotnetInjector)(nil)
var _ ContainerInjector = (*DotnetInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&DotnetInjector{baseInjector{lang: "dotnet"}})
}

type DotnetInjector struct {
	baseInjector
}

func (i *DotnetInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	return i.InjectContainer(ctx, inst, ns, pod, pod.Spec.Containers[0].Name)
}

func (i *DotnetInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}

	if err := validateContainerEnv(container.Env, envDotnetCoreClrEnableProfiling, envDotnetCoreClrProfiler, envDotnetCoreClrProfilerPath, envDotnetNewrelicHome); err != nil {
		return corev1.Pod{}, err
	}

	setEnvVar(container, envDotnetCoreClrEnableProfiling, dotnetCoreClrEnableProfilingEnabled, false, "")
	setEnvVar(container, envDotnetCoreClrProfiler, dotnetCoreClrProfilerID, false, "")
	setEnvVar(container, envDotnetCoreClrProfilerPath, dotnetCoreClrProfilerPath, false, "")
	setEnvVar(container, envDotnetNewrelicHome, dotnetNewrelicHomePath, false, "")
	if v, _ := getValueFromEnv(container.Env, envDotnetCoreClrEnableProfiling); v != dotnetCoreClrEnableProfilingEnabled {
		return corev1.Pod{}, errUnableToConfigureEnv
	}
	if v, _ := getValueFromEnv(container.Env, envDotnetCoreClrProfiler); v != dotnetCoreClrProfilerID {
		return corev1.Pod{}, errUnableToConfigureEnv
	}
	if v, _ := getValueFromEnv(container.Env, envDotnetCoreClrProfilerPath); v != dotnetCoreClrProfilerPath {
		return corev1.Pod{}, errUnableToConfigureEnv
	}
	if v, _ := getValueFromEnv(container.Env, envDotnetNewrelicHome); v != dotnetNewrelicHomePath {
		return corev1.Pod{}, errUnableToConfigureEnv
	}
	setContainerEnvFromInst(container, inst)

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/newrelic-instrumentation",
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, dotnetInitContainerName) {
		if isPodVolumeMissing(pod, volumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		newContainer := corev1.Container{
			Name:    dotnetInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
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
