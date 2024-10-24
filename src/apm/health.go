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
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

const (
	envHealthFleetControlFile  = "NEW_RELIC_FLEET_CONTROL_HEALTH_FILE"
	envHealthListenPort        = "NEW_RELIC_SIDECAR_LISTEN_PORT"
	envHealthTimeout           = "NEW_RELIC_SIDECAR_TIMEOUT_DURATION"
	envHealthDebug             = "DEBUG"
	healthSidecarContainerName = "newrelic-apm-health"
	healthVolumeName           = "newrelic-apm-health"
)

var healthDefaultEnvMap = map[string]string{
	envHealthDebug:      "false",
	envHealthListenPort: "6194",
	envHealthTimeout:    "1s",
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

	err := validateContainerEnv(container.Env, envHealthFleetControlFile, envHealthListenPort, envHealthTimeout, envHealthDebug)
	if err != nil {
		return pod, err
	}

	heathFileIdx := getIndexOfEnv(container.Env, envHealthFleetControlFile)
	if heathFileIdx == -1 {
		return pod, fmt.Errorf("missing required %q env variable", envHealthFleetControlFile)
	}
	healthMountPath := filepath.Base(container.Env[heathFileIdx].Value)
	if healthMountPath == "" {
		return pod, fmt.Errorf("env variable %q configured incorrectly.  requires a full path", envHealthFleetControlFile)
	}

	// inject Health instrumentation spec env vars.
	for _, env := range inst.Spec.Agent.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	//set defaults
	for defaultEnvKey, defaultEnvValue := range healthDefaultEnvMap {
		if idx := getIndexOfEnv(container.Env, defaultEnvKey); idx == -1 {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  defaultEnvKey,
				Value: defaultEnvValue,
			})
		}
	}

	//validate env values
	healthListenPortIdx := getIndexOfEnv(container.Env, envHealthListenPort)
	healthListenPort, err := strconv.Atoi(container.Env[healthListenPortIdx].Value)
	if err != nil {
		return pod, fmt.Errorf("invalid health listen port %q > %w", container.Env[healthListenPortIdx].Value, err)
	}
	if healthListenPort > 65535 || healthListenPort < 1 {
		return pod, fmt.Errorf("invalid health listen port %q, must be between 1-65535 (inclusive)", container.Env[healthListenPortIdx].Value)
	}

	healthTimeoutIdx := getIndexOfEnv(container.Env, envHealthTimeout)
	if _, err = time.ParseDuration(container.Env[healthTimeoutIdx].Value); err != nil {
		return pod, fmt.Errorf("invalid health timeout %q > %w", container.Env[healthTimeoutIdx].Value, err)
	}

	healthDebugIdx := getIndexOfEnv(container.Env, envHealthDebug)
	if _, err = strconv.ParseBool(container.Env[healthDebugIdx].Value); err != nil {
		return pod, fmt.Errorf("invalid debug value %q > %w", container.Env[healthDebugIdx].Value, err)
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
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:          healthSidecarContainerName,
			Image:         inst.Spec.Agent.Image,
			RestartPolicy: &restartAlways,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      healthVolumeName,
				MountPath: healthMountPath,
			}},
			Ports: []corev1.ContainerPort{
				{ContainerPort: int32(healthListenPort)},
			},
		})
	}

	return pod, nil
}
