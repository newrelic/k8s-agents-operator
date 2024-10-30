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
	"slices"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

const (
	envHealthFleetControlFile  = "NEW_RELIC_FLEET_CONTROL_HEALTH_FILE"
	envHealthListenPort        = "NEW_RELIC_SIDECAR_LISTEN_PORT"
	envHealthTimeout           = "NEW_RELIC_SIDECAR_TIMEOUT_DURATION"
	healthSidecarContainerName = "newrelic-apm-health"
	healthVolumeName           = "newrelic-apm-health"
)

var (
	defaultHealthListenPort = 6194
	defaultHealthTimeout    = time.Second
)

var healthDefaultEnvMap = []corev1.EnvVar{
	{Name: envHealthListenPort, Value: fmt.Sprintf("%d", defaultHealthListenPort)},
	{Name: envHealthTimeout, Value: defaultHealthTimeout.String()},
}

var _ Injector = (*HealthInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&HealthInjector{})
}

type HealthInjector struct {
	baseInjector
}

func (i *HealthInjector) Language() string {
	return "health"
}

func (i *HealthInjector) acceptable(inst v1alpha2.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

func (i *HealthInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	firstContainer := 0
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	err := validateContainerEnv(container.Env, envHealthFleetControlFile, envHealthListenPort, envHealthTimeout)
	if err != nil {
		return pod, err
	}

	var initContainerEnv []corev1.EnvVar

	// copy the env var for the health file
	if idx := getIndexOfEnv(container.Env, envHealthFleetControlFile); idx > -1 {
		initContainerEnv = append(initContainerEnv, container.Env[idx])
	}

	// inject Health instrumentation spec env vars.
	for _, env := range inst.Spec.Agent.Env {
		// configure sidecar specific env vars
		if slices.Contains([]string{envHealthListenPort, envHealthTimeout}, env.Name) {
			if idx := getIndexOfEnv(initContainerEnv, env.Name); idx == -1 {
				initContainerEnv = append(initContainerEnv, env)
			}
			continue
		}
		// configure env vars for both the sidecar and the first container
		if envHealthFleetControlFile == env.Name {
			if idx := getIndexOfEnv(initContainerEnv, env.Name); idx == -1 {
				initContainerEnv = append(initContainerEnv, env)
			}
			if idx := getIndexOfEnv(container.Env, env.Name); idx == -1 {
				container.Env = append(container.Env, env)
			}
			continue
		}
		// configure the remaining env vars for the first container
		if idx := getIndexOfEnv(container.Env, env.Name); idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	heathFileIdx := getIndexOfEnv(container.Env, envHealthFleetControlFile)
	if heathFileIdx == -1 {
		return pod, fmt.Errorf("missing required %q env variable", envHealthFleetControlFile)
	}
	healthMountPath := filepath.Dir(container.Env[heathFileIdx].Value)
	if healthMountPath == "" {
		return pod, fmt.Errorf("env variable %q configured incorrectly.  requires a full path", envHealthFleetControlFile)
	}
	if healthMountPath == "/" {
		return pod, fmt.Errorf("env variable %q configured incorrectly.  cannot be the root", envHealthFleetControlFile)
	}

	// set defaults
	for _, entry := range healthDefaultEnvMap {
		if idx := getIndexOfEnv(container.Env, entry.Name); idx == -1 {
			initContainerEnv = append(initContainerEnv, corev1.EnvVar{
				Name:  entry.Name,
				Value: entry.Value,
			})
		}
	}

	sidecarListenPort := defaultHealthListenPort

	// validate env values
	if healthListenPortIdx := getIndexOfEnv(initContainerEnv, envHealthListenPort); healthListenPortIdx > -1 {
		healthListenPort, err := strconv.Atoi(initContainerEnv[healthListenPortIdx].Value)
		if err != nil {
			return pod, fmt.Errorf("invalid health listen port %q > %w", initContainerEnv[healthListenPortIdx].Value, err)
		}
		if healthListenPort > 65535 || healthListenPort < 1 {
			return pod, fmt.Errorf("invalid health listen port %q, must be between 1-65535 (inclusive)", initContainerEnv[healthListenPortIdx].Value)
		}
		sidecarListenPort = healthListenPort
	}
	if healthTimeoutIdx := getIndexOfEnv(initContainerEnv, envHealthTimeout); healthTimeoutIdx > -1 {
		if _, err = time.ParseDuration(initContainerEnv[healthTimeoutIdx].Value); err != nil {
			return pod, fmt.Errorf("invalid health timeout %q > %w", initContainerEnv[healthTimeoutIdx].Value, err)
		}
	}

	if isContainerVolumeMissing(&pod.Spec.Containers[firstContainer], healthVolumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      healthVolumeName,
			MountPath: healthMountPath,
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, healthSidecarContainerName) {
		if isPodVolumeMissing(pod, healthVolumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: healthVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		restartAlways := corev1.ContainerRestartPolicyAlways
		initContainer := corev1.Container{
			Name:          healthSidecarContainerName,
			Image:         inst.Spec.Agent.Image,
			RestartPolicy: &restartAlways,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      healthVolumeName,
				MountPath: healthMountPath,
			}},
			Ports: []corev1.ContainerPort{
				{ContainerPort: int32(sidecarListenPort)},
			},
			Env: initContainerEnv,
		}

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, initContainer)
	}

	return pod, nil
}
