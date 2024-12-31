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
	"path/filepath"
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

const (
	envHealthFleetControlFilepath = "NEW_RELIC_FLEET_CONTROL_HEALTH_PATH"
	envHealthListenPort           = "NEW_RELIC_SIDECAR_LISTEN_PORT"
	HealthSidecarContainerName    = "newrelic-apm-health-sidecar"
	healthVolumeName              = "newrelic-apm-health-volume"

	HealthInstrumentedAnnotation = "newrelic.com/apm-health"
)

const (
	maxPort = 65535
	minPort = 1
)

var (
	defaultHealthListenPort = 6194
)

var healthDefaultEnv = []corev1.EnvVar{
	{Name: envHealthListenPort, Value: fmt.Sprintf("%d", defaultHealthListenPort)},
}

func (i *baseInjector) injectHealth(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if inst.Spec.HealthAgent.IsEmpty() {
		return pod, nil
	}

	originalPod := pod.DeepCopy()
	firstContainer := 0

	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	var err error
	if err = validateContainerEnv(container.Env, envHealthFleetControlFilepath); err != nil {
		return *originalPod, err
	}

	var sidecarContainerEnv []corev1.EnvVar

	// copy the env var for the health filepath
	if idx := getIndexOfEnv(container.Env, envHealthFleetControlFilepath); idx > -1 {
		sidecarContainerEnv = append(sidecarContainerEnv, container.Env[idx])
	}

	container.Env = i.injectEnvVarsIntoTargetedEnvVars(inst.Spec.HealthAgent.Env, container.Env)
	sidecarContainerEnv = i.injectEnvVarsIntoSidecarEnvVars(inst.Spec.HealthAgent.Env, sidecarContainerEnv)

	var healthMountPath string
	{
		v, _ := getValueFromEnv(container.Env, envHealthFleetControlFilepath)
		if healthMountPath, err = i.validateHealthFilepath(v); err != nil {
			return *originalPod, fmt.Errorf("invalid env value %q for %q > %w", v, envHealthFleetControlFilepath, err)
		}
	}

	// set defaults
	for _, entry := range healthDefaultEnv {
		if idx := getIndexOfEnv(sidecarContainerEnv, entry.Name); idx == -1 {
			sidecarContainerEnv = append(sidecarContainerEnv, corev1.EnvVar{
				Name:  entry.Name,
				Value: entry.Value,
			})
		}
	}

	sidecarListenPort := defaultHealthListenPort
	if v, ok := getValueFromEnv(sidecarContainerEnv, envHealthListenPort); ok {
		sidecarListenPort, err = i.validateHealthListenPort(v)
		if err != nil {
			return *originalPod, fmt.Errorf("invalid env value %q for %q > %w", v, envHealthListenPort, err)
		}
	}

	if isContainerVolumeMissing(&pod.Spec.Containers[firstContainer], healthVolumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      healthVolumeName,
			MountPath: healthMountPath,
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, HealthSidecarContainerName) {
		if isPodVolumeMissing(pod, healthVolumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: healthVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		restartAlways := corev1.ContainerRestartPolicyAlways
		sidecarContainer := corev1.Container{
			Name:          HealthSidecarContainerName,
			Image:         inst.Spec.HealthAgent.Image,
			RestartPolicy: &restartAlways,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      healthVolumeName,
				MountPath: healthMountPath,
			}},
			Ports: []corev1.ContainerPort{
				{ContainerPort: int32(sidecarListenPort)},
			},
			Env: sidecarContainerEnv,
		}

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, sidecarContainer)
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[HealthInstrumentedAnnotation] = "true"

	return pod, nil
}

func (i *baseInjector) injectEnvVarsIntoTargetedEnvVars(instEnvVars []corev1.EnvVar, containerEnvVars []corev1.EnvVar) []corev1.EnvVar {
	for _, env := range instEnvVars {
		if env.Name == envHealthFleetControlFilepath {
			if idx := getIndexOfEnv(containerEnvVars, env.Name); idx == -1 {
				containerEnvVars = append(containerEnvVars, env)
			}
			break
		}
	}
	return containerEnvVars
}

func (i *baseInjector) injectEnvVarsIntoSidecarEnvVars(instEnvVars []corev1.EnvVar, sidecarEnvVars []corev1.EnvVar) []corev1.EnvVar {
	for _, env := range instEnvVars {
		if idx := getIndexOfEnv(sidecarEnvVars, env.Name); idx == -1 {
			sidecarEnvVars = append(sidecarEnvVars, env)
		}
	}
	return sidecarEnvVars
}

func (i *baseInjector) validateHealthListenPort(value string) (int, error) {
	healthListenPort, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid health listen port %q > %w", value, err)
	}
	if healthListenPort > maxPort || healthListenPort < minPort {
		return 0, fmt.Errorf("invalid health listen port %q, must be between %d-%d (inclusive)", value, minPort, maxPort)
	}
	return healthListenPort, nil
}

func (i *baseInjector) validateHealthFilepath(value string) (string, error) {
	fileExtension := filepath.Ext(value)
	if fileExtension != "" {
		return "", fmt.Errorf("invalid mount path %q, cannot have a file extension", value)
	}
	if value == "" {
		return "", fmt.Errorf("invalid mount path %q, cannot be blank", value)
	}
	if value == "/" {
		return "", fmt.Errorf("invalid mount path %q, cannot be root", value)
	}

	return value, nil
}
