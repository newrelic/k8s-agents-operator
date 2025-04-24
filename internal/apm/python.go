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
	"github.com/newrelic/k8s-agents-operator/api/current"
	corev1 "k8s.io/api/core/v1"
)

const (
	envPythonPath           = "PYTHONPATH"
	pythonPathPrefix        = "/newrelic-instrumentation"
	pythonInitContainerName = initContainerName + "-python"
)

var _ Injector = (*PythonInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&PythonInjector{})
}

type PythonInjector struct {
	baseInjector
}

func (i *PythonInjector) Language() string {
	return "python"
}

func (i *PythonInjector) acceptable(inst current.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

func (i *PythonInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	firstContainer := 0
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	if err := validateContainerEnv(container.Env, envPythonPath); err != nil {
		return pod, err
	}
	setEnvVar(container, envPythonPath, pythonPathPrefix, true, ":")
	setContainerEnvFromInst(container, inst)

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

	pod = i.injectNewrelicConfig(ctx, ns, pod, firstContainer, inst.Spec.LicenseKeySecret)

	pod = addAnnotationToPodFromInstrumentationVersion(ctx, pod, inst)

	var err error
	if pod, err = i.injectHealth(ctx, inst, ns, pod, firstContainer, -1); err != nil {
		return pod, err
	}

	return pod, nil
}
