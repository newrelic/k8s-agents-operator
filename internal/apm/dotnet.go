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

	"github.com/newrelic/k8s-agents-operator/api/current"
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

func init() {
	DefaultInjectorRegistry.MustRegister(&DotnetInjector{})
}

type DotnetInjector struct {
	baseInjector
}

func (i *DotnetInjector) Language() string {
	return "dotnet"
}

func (i *DotnetInjector) acceptable(inst current.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

func (i DotnetInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return corev1.Pod{}, err
	}

	firstContainer := 0
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

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

	pod = i.injectNewrelicConfig(ctx, ns, pod, firstContainer, inst.Spec.LicenseKeySecret)

	pod = addAnnotationToPodFromInstrumentationVersion(ctx, pod, inst)

	var err error
	if pod, err = i.injectHealth(ctx, inst, ns, pod, firstContainer, -1); err != nil {
		return corev1.Pod{}, err
	}

	return pod, nil
}
