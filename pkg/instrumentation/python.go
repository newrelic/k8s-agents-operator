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
package instrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic-experimental/newrelic-agent-operator/api/v1alpha1"
)

const (
	envPythonPath           = "PYTHONPATH"
	pythonPathPrefix        = "/newrelic-instrumentation/newrelic/bootstrap"
	pythonPathSuffix        = "/newrelic-instrumentation"
	pythonVolumeName        = volumeName + "-python"
	pythonInitContainerName = initContainerName + "-python"
)

func injectPythonSDK(pythonSpec v1alpha1.Python, pod corev1.Pod, index int) (corev1.Pod, error) {
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[index]

	err := validateContainerEnv(container.Env, envPythonPath)
	if err != nil {
		return pod, err
	}

	// inject Python instrumentation spec env vars.
	for _, env := range pythonSpec.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	idx := getIndexOfEnv(container.Env, envPythonPath)
	if idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envPythonPath,
			Value: fmt.Sprintf("%s:%s", pythonPathPrefix, pythonPathSuffix),
		})
	} else if idx > -1 {
		container.Env[idx].Value = fmt.Sprintf("%s:%s:%s", pythonPathPrefix, container.Env[idx].Value, pythonPathSuffix)
	}

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: "/newrelic-instrumentation",
	})

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			}})

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:    initContainerName,
			Image:   pythonSpec.Image,
			Command: []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
		})
	}
	return pod, nil
}
