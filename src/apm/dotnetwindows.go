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
	//DotNet Framework
	envDotnetFrameworkWindowsClrEnableProfiling     = "COR_ENABLE_PROFILING"
	envDotnetFrameworkWindowsClrProfiler            = "COR_PROFILER"
	envDotnetFrameworkWindowsClrProfilerPath        = "COR_PROFILER_PATH"
	envDotnetFrameworkWindowsNewrelicHome           = "NEWRELIC_HOME"
	dotnetFrameworkWindowsClrEnableProfilingEnabled = "1"
	dotnetFrameworkWindowsClrProfilerID             = "{71DA0A04-7777-4EC6-9643-7D28B46A8A41}"
	dotnetFrameworkWindowsClrProfilerPath           = "C:\\newrelic-instrumentation\\netframework\\NewRelic.Profiler.dll"
	dotnetFrameworkWindowsNewrelicHomePath          = "C:\\newrelic-instrumentation\\netframework"

	//DotNet Core
	envDotnetWindowsClrEnableProfiling         = "CORECLR_ENABLE_PROFILING"
	envDotnetWindowsClrProfiler                = "CORECLR_PROFILER"
	envDotnetWindowsClrProfilerPath            = "CORECLR_PROFILER_PATH"
	envDotnetWindowsNewrelicHome               = "CORECLR_NEWRELIC_HOME"
	dotnetCoreWindowsClrEnableProfilingEnabled = "1"
	dotnetCoreWindowsClrProfilerID             = "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"
	dotnetCoreWindowsClrProfilerPath           = "C:\\newrelic-instrumentation\\netcore\\NewRelic.Profiler.dll"
	dotnetCoreWindowsNewrelicHomePath          = "C:\\newrelic-instrumentation\\netcore"
	dotnetWindowsInitContainerName             = initContainerName + "-dotnet-windows"
)

var _ Injector = (*DotnetWindowsInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&DotnetWindowsInjector{})
}

type DotnetWindowsInjector struct {
	baseInjector
}

func (i *DotnetWindowsInjector) Language() string {
	return "dotnet-windows"
}

func (i *DotnetWindowsInjector) acceptable(inst v1alpha2.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

func (i DotnetWindowsInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
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
	if getIndexOfEnv(container.Env, envDotnetWindowsNewrelicHome) > -1 {
		return pod, errors.New("CORECLR_NEWRELIC_HOME environment variable is already set in the container")
	}

	// check if CORECLR_NEWRELIC_HOME env var is already set in the .NET instrumentation spec
	// if it is already set, then we assume that .NET newrelic-instrumentation is already configured for this container
	if getIndexOfEnv(inst.Spec.Agent.Env, envDotnetWindowsNewrelicHome) > -1 {
		return pod, errors.New("CORECLR_NEWRELIC_HOME environment variable is already set in the .NET instrumentation spec")
	}

	// check if NEWRELIC_HOME env var is already set in the container
	// if it is already set, then we assume that .NET newrelic-instrumentation is already configured for this container
	if getIndexOfEnv(container.Env, envDotnetFrameworkWindowsNewrelicHome) > -1 {
		return pod, errors.New("NEWRELIC_HOME environment variable is already set in the container")
	}

	// check if NEWRELIC_HOME env var is already set in the .NET instrumentation spec
	// if it is already set, then we assume that .NET newrelic-instrumentation is already configured for this container
	if getIndexOfEnv(inst.Spec.Agent.Env, envDotnetFrameworkWindowsNewrelicHome) > -1 {
		return pod, errors.New("NEWRELIC_HOME environment variable is already set in the .NET instrumentation spec")
	}

	// inject .NET instrumentation spec env vars.
	for _, env := range inst.Spec.Agent.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	setEnvVar(container, envDotnetWindowsClrEnableProfiling, dotnetCoreWindowsClrEnableProfilingEnabled, false)
	setEnvVar(container, envDotnetWindowsClrProfiler, dotnetCoreWindowsClrProfilerID, false)
	setEnvVar(container, envDotnetWindowsClrProfilerPath, dotnetCoreWindowsClrProfilerPath, false)
	setEnvVar(container, envDotnetWindowsNewrelicHome, dotnetCoreWindowsNewrelicHomePath, false)
	setEnvVar(container, envDotnetFrameworkWindowsClrEnableProfiling, dotnetFrameworkWindowsClrEnableProfilingEnabled, false)
	setEnvVar(container, envDotnetFrameworkWindowsClrProfiler, dotnetFrameworkWindowsClrProfilerID, false)
	setEnvVar(container, envDotnetFrameworkWindowsClrProfilerPath, dotnetFrameworkWindowsClrProfilerPath, false)
	setEnvVar(container, envDotnetFrameworkWindowsNewrelicHome, dotnetFrameworkWindowsNewrelicHomePath, false)

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "C:\\newrelic-instrumentation",
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, dotnetWindowsInitContainerName) {
		if isPodVolumeMissing(pod, volumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:    dotnetWindowsInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"cmd", "/C", "xcopy C:\\instrumentation C:\\newrelic-instrumentation /E /I /H /Y"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "C:\\newrelic-instrumentation",
			}},
		})
	}

	pod = i.injectNewrelicConfig(ctx, inst.Spec.Resource, ns, pod, firstContainer, inst.Spec.LicenseKeySecret)

	return pod, nil
}
