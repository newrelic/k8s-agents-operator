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
	envDotnetFrameworkClrEnableProfiling     = "COR_ENABLE_PROFILING"
	envDotnetFrameworkClrProfiler            = "COR_PROFILER"
	envDotnetFrameworkClrProfilerPath        = "COR_PROFILER_PATH"
	envDotnetFrameworkNewrelicHome           = "NEWRELIC_HOME"
	dotnetFrameworkClrEnableProfilingEnabled = "1"
	dotnetFrameworkClrProfilerID             = "{71DA0A04-7777-4EC6-9643-7D28B46A8A41}"

	windowsEnvDotnetCoreClrEnableProfiling     = "CORECLR_ENABLE_PROFILING"
	windowsEnvDotnetCoreClrProfiler            = "CORECLR_PROFILER"
	windowsEnvDotnetCoreClrProfilerPath        = "CORECLR_PROFILER_PATH"
	windowsEnvDotnetCoreNewrelicHome           = "CORECLR_NEWRELIC_HOME"
	windowsDotnetCoreClrEnableProfilingEnabled = "1"
	windowsDotnetCoreClrProfilerID             = "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"

	windowsEnvDotnetAgentLogPath    = "NEWRELIC_LOG_DIRECTORY"
	windowsEnvDotnetProfilerLogPath = "NEWRELIC_PROFILER_LOG_DIRECTORY"
)

var errUnableToConfigureEnvWindows = errors.New("unable to configure environment variables, they've already been set to different values")

var _ ContainerInjector = (*DotnetWindowsInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&DotnetWindowsInjector{baseInjector{lang: "dotnet-windows2022"}})
	DefaultInjectorRegistry.MustRegister(&DotnetWindowsInjector{baseInjector{lang: "dotnet-windows2025"}})
}

type DotnetWindowsInjector struct {
	baseInjector
}

func (i *DotnetWindowsInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}

	initContainerName := generateContainerName("nri-dotnet--" + containerName) // TODO: Does this need to be dotnet-window and if so are they are other places?
	volumeName := initContainerName
	mountPath := "c:\\" + volumeName // path on c:\ where the volume is mounted
	frameworkClrProfilerPath := mountPath + "\\netframework\\NewRelic.Profiler.dll"
	coreClrProfilerPath := mountPath + "\\netcore\\NewRelic.Profiler.dll"
	frameworkNewrelicHomePath := mountPath + "\\netframework"
	coreNewrelicHomePath := mountPath + "\\netcore"

	// Logging
	logsPath := mountPath + "\\Logs"
	setEnvVar(container, windowsEnvDotnetAgentLogPath, logsPath, false, "")
	setEnvVar(container, windowsEnvDotnetProfilerLogPath, logsPath, false, "")

	// Framework env vars validation
	if err := validateContainerEnv(container.Env, envDotnetFrameworkClrEnableProfiling, envDotnetFrameworkClrProfiler, envDotnetFrameworkClrProfilerPath, envDotnetFrameworkNewrelicHome); err != nil {
		return corev1.Pod{}, err
	}

	setEnvVar(container, envDotnetFrameworkClrEnableProfiling, dotnetFrameworkClrEnableProfilingEnabled, false, "")
	setEnvVar(container, envDotnetFrameworkClrProfiler, dotnetFrameworkClrProfilerID, false, "")
	setEnvVar(container, envDotnetFrameworkClrProfilerPath, frameworkClrProfilerPath, false, "")
	setEnvVar(container, envDotnetFrameworkNewrelicHome, frameworkNewrelicHomePath, false, "")
	if v, _ := getValueFromEnv(container.Env, envDotnetFrameworkClrEnableProfiling); v != dotnetFrameworkClrEnableProfilingEnabled {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}
	if v, _ := getValueFromEnv(container.Env, envDotnetFrameworkClrProfiler); v != dotnetFrameworkClrProfilerID {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}
	if v, _ := getValueFromEnv(container.Env, envDotnetFrameworkClrProfilerPath); v != frameworkClrProfilerPath {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}
	if v, _ := getValueFromEnv(container.Env, envDotnetFrameworkNewrelicHome); v != frameworkNewrelicHomePath {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}

	// Core env vars validation
	if err := validateContainerEnv(container.Env, windowsEnvDotnetCoreClrEnableProfiling, windowsEnvDotnetCoreClrProfiler, windowsEnvDotnetCoreClrProfilerPath, windowsEnvDotnetCoreNewrelicHome); err != nil {
		return corev1.Pod{}, err
	}

	setEnvVar(container, windowsEnvDotnetCoreClrEnableProfiling, windowsDotnetCoreClrEnableProfilingEnabled, false, "")
	setEnvVar(container, windowsEnvDotnetCoreClrProfiler, windowsDotnetCoreClrProfilerID, false, "")
	setEnvVar(container, windowsEnvDotnetCoreClrProfilerPath, coreClrProfilerPath, false, "")
	setEnvVar(container, windowsEnvDotnetCoreNewrelicHome, coreNewrelicHomePath, false, "")
	if v, _ := getValueFromEnv(container.Env, windowsEnvDotnetCoreClrEnableProfiling); v != windowsDotnetCoreClrEnableProfilingEnabled {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}
	if v, _ := getValueFromEnv(container.Env, windowsEnvDotnetCoreClrProfiler); v != windowsDotnetCoreClrProfilerID {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}
	if v, _ := getValueFromEnv(container.Env, windowsEnvDotnetCoreClrProfilerPath); v != coreClrProfilerPath {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}
	if v, _ := getValueFromEnv(container.Env, windowsEnvDotnetCoreNewrelicHome); v != coreNewrelicHomePath {
		return corev1.Pod{}, errUnableToConfigureEnvWindows
	}

	setContainerEnvFromInst(container, inst)

	addPodVolumeIfMissing(&pod, volumeName)
	addContainerVolumeIfMissing(container, volumeName, mountPath)

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(&pod, initContainerName) {
		newContainer := corev1.Container{
			Name:            initContainerName,
			Image:           inst.Spec.Agent.Image,
			ImagePullPolicy: inst.Spec.Agent.ImagePullPolicy,
			Command:         []string{"cmd", "/C", "xcopy C:\\instrumentation " + mountPath + " /E /I /H /Y /F"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: mountPath,
			}},
			Resources:       *inst.Spec.Agent.Resources.DeepCopy(),
			SecurityContext: inst.Spec.Agent.SecurityContext.DeepCopy(),
		}
		addContainer(isTargetInitContainer, containerName, &pod, newContainer)

		// re get container, it's address in memory likely changed, since appending can allocate a new slice
		container, _ = util.GetContainerByNameFromPod(&pod, containerName)
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
