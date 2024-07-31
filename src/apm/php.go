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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha1"
)

const (
	envIniScanDirKey = "PHP_INI_SCAN_DIR"
	envIniScanDirVal = ":/newrelic-instrumentation/php-agent/ini"
)

var phpApiMap = map[string]string{
	"7.2": "20170718",
	"7.3": "20180731",
	"7.4": "20190902",
	"8.0": "20200930",
	"8.1": "20210902",
	"8.2": "20220829",
	"8.3": "20230831",
}

func InjectPhpagent(phpSpec v1alpha1.Php, pod corev1.Pod, index int) (corev1.Pod, error) {
	// exit early if we're missing mandatory annotations
	phpVer, ok := pod.Annotations[annotationPhpVersion]
	if !ok {
		return pod, errors.New("missing php version annotation")
	}

	apiNum, ok := phpApiMap[phpVer]
	if !ok {
		return pod, errors.New("invalid php version")
	}

	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[index]

	// inject PHP instrumentation spec env vars.
	for _, env := range phpSpec.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	// define instrumentation artifact volume
	instVolume := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: "/newrelic-instrumentation",
	}

	setPhpEnvVar(container, envIniScanDirKey, envIniScanDirVal, true)

	container.VolumeMounts = append(container.VolumeMounts, instVolume)

	// We only inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod) {
		initContainer := corev1.Container{
			Name:    initContainerName,
			Image:   phpSpec.Image,
			Command: []string{"/bin/sh"},
			Args: []string{
				"-c", "cp -a /instrumentation/. /newrelic-instrumentation/ && /newrelic-instrumentation/k8s-php-install.sh " + apiNum + " && /newrelic-instrumentation/nr_env_to_ini.sh",
			},
			VolumeMounts: []corev1.VolumeMount{instVolume},
		}

		// inject the spec env vars into the initcontainer in order to construct the ini file
		for _, env := range container.Env {
			initContainer.Env = append(initContainer.Env, env)
		}

		// inject the license key secret
		optional := true
		initContainer.Env = append(initContainer.Env, corev1.EnvVar{
			Name: "NEW_RELIC_LICENSE_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"},
					Key:                  "new_relic_license_key",
					Optional:             &optional,
				},
			},
		})

		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			}})

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, initContainer)
	}

	return pod, nil
}

// setPhpEnvVar function sets env var to the container if not exist already.
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
