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
	"net/url"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/util"
)

const (
	envAgentControlEnabled                = "NEW_RELIC_AGENT_CONTROL_ENABLED"
	envAgentControlHealthDeliveryLocation = "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION"
	envHealthListenPort                   = "NEW_RELIC_SIDECAR_LISTEN_PORT"

	HealthInstrumentedAnnotation = "newrelic.com/apm-health"
)

const (
	maxPort = 65535
	minPort = 1
)

var (
	defaultHealthListenPort = 6194
)

type HealthInjector struct {
	baseInjector
}

func NewHealthInjector() *HealthInjector {
	return &HealthInjector{baseInjector{lang: "health"}}
}

func (i *HealthInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string, healthPort int, healthPath string) (corev1.Pod, error) {
	if inst.Spec.HealthAgent.IsEmpty() {
		return pod, nil
	}
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}
	if isTargetInitContainer {
		return corev1.Pod{}, fmt.Errorf("container %q is a init container, which can't have a health sidecar", containerName)
	}

	if err := validateContainerEnv(container.Env, envAgentControlHealthDeliveryLocation, envHealthListenPort); err != nil {
		return corev1.Pod{}, err
	}

	healthSidecarContainerName := GenerateContainerName("nri-health--" + container.Name)
	healthVolumeName := healthSidecarContainerName
	healthMountPath := "/" + healthSidecarContainerName
	if healthPath == "" {
		healthPath = "file://" + healthMountPath
	}

	if healthPath != "" {
		u, _ := url.Parse(healthPath)
		parts := strings.SplitN(u.Path, "/", 3)
		if len(parts) < 2 {
			return corev1.Pod{}, fmt.Errorf("volume mount path is incorrect for container %q", containerName)
		}
		healthMountPath = "/" + parts[1]
	}

	if _, err := i.validateHealthFilepath(healthPath); err != nil {
		return corev1.Pod{}, fmt.Errorf("invalid env value %q for %q > %w", healthPath, envAgentControlHealthDeliveryLocation, err)
	}

	if healthPort == 0 {
		healthPort = defaultHealthListenPort
	}
	_, err := i.validateHealthListenPort(healthPort)
	if err != nil {
		return corev1.Pod{}, fmt.Errorf("invalid env value %d for %q > %w", healthPort, envHealthListenPort, err)
	}

	var sidecarContainerEnv []corev1.EnvVar

	if idx := getIndexOfEnv(container.Env, envAgentControlHealthDeliveryLocation); idx > -1 {
		container.Env[idx].Value = healthPath
	} else {
		container.Env = append(container.Env, corev1.EnvVar{Name: envAgentControlHealthDeliveryLocation, Value: healthPath})
	}
	if idx := getIndexOfEnv(sidecarContainerEnv, envAgentControlHealthDeliveryLocation); idx > -1 {
		sidecarContainerEnv[idx].Value = healthPath
	} else {
		sidecarContainerEnv = append(sidecarContainerEnv, corev1.EnvVar{Name: envAgentControlHealthDeliveryLocation, Value: healthPath})
	}
	if idx := getIndexOfEnv(sidecarContainerEnv, envHealthListenPort); idx > -1 {
		sidecarContainerEnv[idx].Value = fmt.Sprintf("%d", healthPort)
	} else {
		sidecarContainerEnv = append(sidecarContainerEnv, corev1.EnvVar{Name: envHealthListenPort, Value: fmt.Sprintf("%d", healthPort)})
	}

	// copy the env var for the health filepath
	if idx := getIndexOfEnv(container.Env, envAgentControlEnabled); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envAgentControlEnabled,
			Value: "true",
		})
	}

	sidecarContainerEnv = i.injectEnvVarsIntoSidecarEnvVars(inst.Spec.HealthAgent.Env, sidecarContainerEnv)

	if isContainerVolumeMissing(container, healthVolumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      healthVolumeName,
			MountPath: healthMountPath,
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(&pod, healthSidecarContainerName) {
		if isPodVolumeMissing(&pod, healthVolumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: healthVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		restartAlways := corev1.ContainerRestartPolicyAlways
		sidecarContainer := corev1.Container{
			Name:            healthSidecarContainerName,
			Image:           inst.Spec.HealthAgent.Image,
			ImagePullPolicy: inst.Spec.HealthAgent.ImagePullPolicy,
			RestartPolicy:   &restartAlways,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      healthVolumeName,
				MountPath: healthMountPath,
			}},
			Ports: []corev1.ContainerPort{
				{ContainerPort: int32(healthPort)},
			},
			Env:             sidecarContainerEnv,
			Resources:       *inst.Spec.HealthAgent.Resources.DeepCopy(),
			SecurityContext: inst.Spec.HealthAgent.SecurityContext.DeepCopy(),
		}

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, sidecarContainer)
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[HealthInstrumentedAnnotation] = "true"

	return pod, nil
}

func (i *baseInjector) injectEnvVarsIntoSidecarEnvVars(instEnvVars []corev1.EnvVar, sidecarEnvVars []corev1.EnvVar) []corev1.EnvVar {
	for _, env := range instEnvVars {
		if idx := getIndexOfEnv(sidecarEnvVars, env.Name); idx == -1 {
			sidecarEnvVars = append(sidecarEnvVars, env)
		}
	}
	return sidecarEnvVars
}

func (i *baseInjector) validateHealthListenPort(value int) (int, error) {
	if value > maxPort || value < minPort {
		return 0, fmt.Errorf("invalid health listen port %d, must be between %d-%d (inclusive)", value, minPort, maxPort)
	}
	return value, nil
}

func (i *baseInjector) validateHealthFilepath(value string) (string, error) {
	originalValue := value
	if !strings.HasPrefix(value, "file://") {
		return "", fmt.Errorf("invalid health path %q, must be file URI", originalValue)
	}
	value = strings.TrimPrefix(value, "file://")
	if value[0] != '/' {
		return "", fmt.Errorf("invalid health path %q, hostname must be blank", originalValue)
	}
	if fileExtension := filepath.Ext(value); fileExtension != "" {
		return "", fmt.Errorf("invalid mount path %q, cannot have a file extension", originalValue)
	}
	if value == "/" {
		return "", fmt.Errorf("invalid mount path %q, cannot be root", originalValue)
	}
	return value, nil
}
