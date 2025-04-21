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
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

const (
	envAgentControlEnabled                = "NEW_RELIC_AGENT_CONTROL_ENABLED"
	envAgentControlHealthDeliveryLocation = "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION"
	envHealthListenPort                   = "NEW_RELIC_SIDECAR_LISTEN_PORT"
	HealthSidecarContainerName            = "newrelic-apm-health-sidecar"
	healthVolumeName                      = "newrelic-apm-health-volume"

	HealthInstrumentedAnnotation = "newrelic.com/apm-health"
)

const (
	maxPort = 65535
	minPort = 1
)

var (
	defaultHealthListenPort       = 6194
	defaultHealthDeliveryLocation = "file:///newrelic/apm/health"
)

var healthDefaultEnv = []corev1.EnvVar{
	{Name: envAgentControlHealthDeliveryLocation, Value: defaultHealthDeliveryLocation},
	{Name: envHealthListenPort, Value: fmt.Sprintf("%d", defaultHealthListenPort)},
}

// Deprecated: use injectHealthWithContainer instead
// injectHealth used to inject health based on container or init container index
func (i *baseInjector) injectHealth(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, agentContainerIndex int, agentInitContainerIndex int) (corev1.Pod, error) {
	// caller checks if there is at least one container.
	var container *corev1.Container
	if agentContainerIndex > -1 && agentContainerIndex < len(pod.Spec.Containers) {
		container = &pod.Spec.Containers[agentContainerIndex]
	} else if agentInitContainerIndex > -1 && agentContainerIndex < len(pod.Spec.InitContainers) {
		container = &pod.Spec.InitContainers[agentInitContainerIndex]
	} else {
		return pod, nil
	}
	return i.injectHealthWithContainer(ctx, inst, ns, pod, container)
}

// injectHealthWithContainer used to inject the health container (sidecar) and mounts requires, among other things
func (i *baseInjector) injectHealthWithContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, container *corev1.Container) (corev1.Pod, error) {
	if inst.Spec.HealthAgent.IsEmpty() {
		return pod, nil
	}
	if container == nil {
		return pod, nil
	}

	originalPod := pod.DeepCopy()

	var err error
	if err = validateContainerEnv(container.Env, envAgentControlHealthDeliveryLocation); err != nil {
		return *originalPod, err
	}

	var sidecarContainerEnv []corev1.EnvVar

	// copy the env var for the health filepath
	if idx := getIndexOfEnv(container.Env, envAgentControlHealthDeliveryLocation); idx > -1 {
		sidecarContainerEnv = append(sidecarContainerEnv, container.Env[idx])
	}
	// copy the env var for the health filepath
	if idx := getIndexOfEnv(container.Env, envAgentControlEnabled); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envAgentControlEnabled,
			Value: "true",
		})
	}

	container.Env = i.injectEnvVarsIntoTargetedEnvVars(inst.Spec.HealthAgent.Env, container.Env)
	sidecarContainerEnv = i.injectEnvVarsIntoSidecarEnvVars(inst.Spec.HealthAgent.Env, sidecarContainerEnv)

	// set defaults to sidecar and container env vars
	for _, entry := range healthDefaultEnv {
		if idx := getIndexOfEnv(sidecarContainerEnv, entry.Name); idx == -1 {
			sidecarContainerEnv = append(sidecarContainerEnv, corev1.EnvVar{
				Name:  entry.Name,
				Value: entry.Value,
			})
		}
		if entry.Name == envAgentControlHealthDeliveryLocation {
			if idx := getIndexOfEnv(container.Env, entry.Name); idx == -1 {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  entry.Name,
					Value: entry.Value,
				})
			}
		}
	}

	healthMountPath := defaultHealthDeliveryLocation
	if v, ok := getValueFromEnv(sidecarContainerEnv, envAgentControlHealthDeliveryLocation); ok && v != "" {
		if healthMountPath, err = i.validateHealthFilepath(v); err != nil {
			return *originalPod, fmt.Errorf("invalid env value %q for %q > %w", v, envAgentControlHealthDeliveryLocation, err)
		}
	}

	sidecarListenPort := defaultHealthListenPort
	if v, ok := getValueFromEnv(sidecarContainerEnv, envHealthListenPort); ok {
		sidecarListenPort, err = i.validateHealthListenPort(v)
		if err != nil {
			return *originalPod, fmt.Errorf("invalid env value %q for %q > %w", v, envHealthListenPort, err)
		}
	}

	if isContainerVolumeMissing(container, healthVolumeName) {
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
		if env.Name == envAgentControlHealthDeliveryLocation {
			if idx := getIndexOfEnv(containerEnvVars, env.Name); idx == -1 {
				if env.Value != "" {
					containerEnvVars = append(containerEnvVars, env)
				}
			}
			break
		}
	}
	return containerEnvVars
}

func (i *baseInjector) injectEnvVarsIntoSidecarEnvVars(instEnvVars []corev1.EnvVar, sidecarEnvVars []corev1.EnvVar) []corev1.EnvVar {
	for _, env := range instEnvVars {
		if idx := getIndexOfEnv(sidecarEnvVars, env.Name); idx == -1 {
			if env.Value != "" {
				sidecarEnvVars = append(sidecarEnvVars, env)
			}
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
	originalValue := value
	if !strings.HasPrefix(value, "file://") {
		return "", fmt.Errorf("invalid health path %q, must be file URI", originalValue)
	}
	value = strings.TrimPrefix(value, "file://")
	if value[0] != '/' {
		return "", fmt.Errorf("invalid health path %q, hostname nmust be blank", originalValue)
	}
	if fileExtension := filepath.Ext(value); fileExtension != "" {
		return "", fmt.Errorf("invalid mount path %q, cannot have a file extension", originalValue)
	}
	if value == "/" {
		return "", fmt.Errorf("invalid mount path %q, cannot be root", originalValue)
	}
	return value, nil
}
