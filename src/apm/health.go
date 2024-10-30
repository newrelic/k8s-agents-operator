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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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

var healthDefaultEnv = []corev1.EnvVar{
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

	originalPod := pod.DeepCopy()
	firstContainer := 0
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	err := validateContainerEnv(container.Env, envHealthFleetControlFile, envHealthListenPort, envHealthTimeout)
	if err != nil {
		return *originalPod, err
	}

	var initContainerEnv []corev1.EnvVar

	// copy the env var for the health file
	if idx := getIndexOfEnv(container.Env, envHealthFleetControlFile); idx > -1 {
		initContainerEnv = append(initContainerEnv, container.Env[idx])
	}

	i.injectEnvVarsIntoTargetedEnvVars(inst.Spec.Agent.Env, &container.Env)
	i.injectEnvVarsIntoSidecarEnvVars(inst.Spec.Agent.Env, &initContainerEnv)

	var healthMountPath string
	{
		v, _ := i.getValueFromEnvVars(container.Env, envHealthFleetControlFile)
		if healthMountPath, err = i.validateHealthFile(v); err != nil {
			return *originalPod, fmt.Errorf("invalid env value %q for %q > %w", v, envHealthFleetControlFile, err)
		}
	}

	// set defaults
	for _, entry := range healthDefaultEnv {
		if idx := getIndexOfEnv(container.Env, entry.Name); idx == -1 {
			initContainerEnv = append(initContainerEnv, corev1.EnvVar{
				Name:  entry.Name,
				Value: entry.Value,
			})
		}
	}

	// validate env values
	sidecarListenPort := defaultHealthListenPort
	if v, ok := i.getValueFromEnvVars(initContainerEnv, envHealthListenPort); ok {
		sidecarListenPort, err = i.validateHealthListenPort(v)
		if err != nil {
			return *originalPod, fmt.Errorf("invalid env value %q for %q > %w", v, envHealthListenPort, err)
		}
	}
	if v, ok := i.getValueFromEnvVars(initContainerEnv, envHealthTimeout); ok {
		if _, err = i.validateHealthTimeout(v); err != nil {
			return *originalPod, fmt.Errorf("invalid env value %q for %q > %w", v, envHealthTimeout, err)
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

func (i *HealthInjector) injectEnvVarsIntoTargetedEnvVars(instEnvVars []corev1.EnvVar, containerEnvVars *[]corev1.EnvVar) {
	for _, env := range instEnvVars {
		// skip configuring sidecar specific env vars
		if slices.Contains([]string{envHealthListenPort, envHealthTimeout}, env.Name) {
			continue
		}

		// configure the remaining env vars for the targeted container
		if idx := getIndexOfEnv(*containerEnvVars, env.Name); idx == -1 {
			*containerEnvVars = append(*containerEnvVars, env)
		}
	}
}

func (i *HealthInjector) injectEnvVarsIntoSidecarEnvVars(instEnvVars []corev1.EnvVar, sidecarEnvVars *[]corev1.EnvVar) {
	for _, env := range instEnvVars {
		if slices.Contains([]string{envHealthListenPort, envHealthTimeout, envHealthFleetControlFile}, env.Name) {
			// configure sidecar specific env vars
			if idx := getIndexOfEnv(*sidecarEnvVars, env.Name); idx == -1 {
				*sidecarEnvVars = append(*sidecarEnvVars, env)
			}
		}
	}
}

func (i *HealthInjector) getValueFromEnvVars(envVars []corev1.EnvVar, name string) (string, bool) {
	for _, env := range envVars {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

func (i *HealthInjector) validateHealthListenPort(value string) (int, error) {
	healthListenPort, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid health listen port %q > %w", value, err)
	}
	if healthListenPort > 65535 || healthListenPort < 1 {
		return 0, fmt.Errorf("invalid health listen port %q, must be between 1-65535 (inclusive)", value)
	}
	return healthListenPort, nil
}

func (i *HealthInjector) validateHealthTimeout(value string) (time.Duration, error) {
	t, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q > %w", value, err)
	}
	return t, nil
}

func (i *HealthInjector) validateHealthFile(value string) (string, error) {
	healthMountPath, healthMountFile := filepath.Split(value)
	if len(healthMountPath) >= 2 {
		healthMountPath = strings.TrimRight(healthMountPath, string(os.PathSeparator))
	}
	if healthMountPath == "" {
		return "", fmt.Errorf("invalid mount path %q from value %q, cannot be blank", healthMountPath, value)
	}
	if healthMountPath == "/" {
		return "", fmt.Errorf("invalid mount path %q from value %q, cannot be root", healthMountPath, value)
	}
	if healthMountFile == "" {
		return "", fmt.Errorf("invalid mount file %q from value %q, cannot be blank", healthMountFile, value)
	}
	return healthMountPath, nil
}
