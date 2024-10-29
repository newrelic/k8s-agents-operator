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

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
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

var _ Injector = (*DotnetInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&DotnetInjector{})
}

type DotnetInjector struct {
	baseInjector
}

func (i *DotnetInjector) Language() string {
	return "dotnet"
}

func (i *DotnetInjector) acceptable(inst v1alpha2.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

func (i DotnetInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	firstContainer := 0
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	// check if CORECLR_NEWRELIC_HOME env var is already set in the container
	// if it is already set, then we assume that .NET newrelic-instrumentation is already configured for this container
	if getIndexOfEnv(container.Env, envDotnetNewrelicHome) > -1 {
		return pod, errors.New("CORECLR_NEWRELIC_HOME environment variable is already set in the container")
	}

	// check if CORECLR_NEWRELIC_HOME env var is already set in the .NET instrumentation spec
	// if it is already set, then we assume that .NET newrelic-instrumentation is already configured for this container
	if getIndexOfEnv(inst.Spec.Agent.Env, envDotnetNewrelicHome) > -1 {
		return pod, errors.New("CORECLR_NEWRELIC_HOME environment variable is already set in the .NET instrumentation spec")
	}

	// inject .NET instrumentation spec env vars.
	for _, env := range inst.Spec.Agent.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	setEnvVar(container, envDotnetCoreClrEnableProfiling, dotnetCoreClrEnableProfilingEnabled, false)
	setEnvVar(container, envDotnetCoreClrProfiler, dotnetCoreClrProfilerID, false)
	setEnvVar(container, envDotnetCoreClrProfilerPath, dotnetCoreClrProfilerPath, false)
	setEnvVar(container, envDotnetNewrelicHome, dotnetNewrelicHomePath, false)

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

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:    dotnetInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
		})
	}

	pod = i.injectNewrelicConfig(ctx, inst.Spec.Resource, ns, pod, firstContainer, inst.Spec.LicenseKeySecret)

	return pod, nil
}
