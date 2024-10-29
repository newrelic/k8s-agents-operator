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

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
)

const (
	envPythonPath           = "PYTHONPATH"
	pythonPathPrefix        = "/newrelic-instrumentation"
	pythonInitContainerName = initContainerName + "-python"
)

var _ Injector = (*GoInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&PythonInjector{})
}

type PythonInjector struct {
	baseInjector
}

func (i *PythonInjector) Language() string {
	return "python"
}

func (i *PythonInjector) acceptable(inst v1alpha2.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

func (i *PythonInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	firstContainer := 0
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	err := validateContainerEnv(container.Env, envPythonPath)
	if err != nil {
		return pod, err
	}

	// inject Python instrumentation spec env vars.
	for _, env := range inst.Spec.Agent.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	idx := getIndexOfEnv(container.Env, envPythonPath)
	if idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envPythonPath,
			Value: pythonPathPrefix,
		})
	} else if idx > -1 {
		container.Env[idx].Value = fmt.Sprintf("%s:%s", pythonPathPrefix, container.Env[idx].Value)
	}

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/newrelic-instrumentation",
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, pythonInitContainerName) {
		if isPodVolumeMissing(pod, volumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:    pythonInitContainerName,
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
