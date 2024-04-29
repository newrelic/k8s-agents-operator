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
	envPhpsymbolicOption      = "NR_INSTALL_USE_CP_NOT_LN"
	phpSymbolicOptionArgument = "1"
	envPhpSilentOption        = "NR_INSTALL_SILENT"
	phpSilentOptionArgument   = "1"
	phpInitContainerName      = initContainerName + "-php"
	phpVolumeName             = volumeName + "-php"
	phpInstallArgument        = "/newrelic-instrumentation/newrelic-install install && sed -i -e \"s/PHP Application/$NEW_RELIC_APP_NAME/g; s/REPLACE_WITH_REAL_KEY/$NEW_RELIC_LICENSE_KEY/g\" /usr/local/etc/php/conf.d/newrelic.ini"
)

func injectPhpagent(phpSpec v1alpha1.Php, pod corev1.Pod, index int) (corev1.Pod, error) {
	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[index]

	// inject PHP instrumentation spec env vars.
	for _, env := range phpSpec.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	const (
		phpConcatEnvValues = false
		concatEnvValues    = true
	)

	setPhpEnvVar(container, envPhpsymbolicOption, phpSymbolicOptionArgument, phpConcatEnvValues)

	setPhpEnvVar(container, envPhpSilentOption, phpSilentOptionArgument, phpConcatEnvValues)

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
			Image:   phpSpec.Image,
			Command: []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
		})
	}

	// Continue with the function regardless of whether annotationPhpExecCmd is present or not
	execCmd, ok := pod.Annotations[annotationPhpExecCmd]
	if ok {
		// Add phpInstallArgument to the command field.
		container.Command = append(container.Command, "/bin/sh", "-c", phpInstallArgument+" && "+execCmd)
	} else {
		container.Command = append(container.Command, "/bin/sh", "-c", phpInstallArgument)
	}

	return pod, nil
}

// setDotNetEnvVar function sets env var to the container if not exist already.
// value of concatValues should be set to true if the env var supports multiple values separated by :.
// If it is set to false, the original container's env var value has priority.
func setPhpEnvVar(container *corev1.Container, envVarName string, envVarValue string, concatValues bool) {
	idx := getIndexOfEnv(container.Env, envVarName)
	if idx < 0 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envVarName,
			Value: envVarValue,
		})
		return
	}
	if concatValues {
		container.Env[idx].Value = fmt.Sprintf("%s:%s", container.Env[idx].Value, envVarValue)
	}
}
