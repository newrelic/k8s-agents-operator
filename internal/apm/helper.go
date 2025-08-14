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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

const LicenseKey = "new_relic_license_key"

const (
	maxContainerNameLength = 63
	hashLength             = 7
)

const (
	EnvNewRelicAppName                   = "NEW_RELIC_APP_NAME"
	EnvNewRelicK8sOperatorEnabled        = "NEW_RELIC_K8S_OPERATOR_ENABLED"
	EnvNewRelicLabels                    = "NEW_RELIC_LABELS"
	EnvNewRelicLicenseKey                = "NEW_RELIC_LICENSE_KEY"
	DescK8sAgentOperatorVersionLabelName = "newrelic-k8s-agents-operator-version"
)

const instrumentationVersionAnnotation = "newrelic.com/instrumentation-versions"

func getContainerIndex(pod corev1.Pod, containerName string) int {
	for i, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return i
		}
	}
	return -1
}

func getInitContainerIndex(pod *corev1.Pod, initContainerName string) int {
	for i, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == initContainerName {
			return i
		}
	}
	return -1
}

func isInitContainerMissing(pod *corev1.Pod, initContainerName string) bool {
	for _, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == initContainerName {
			return false
		}
	}
	return true
}

func isPodVolumeMissing(pod *corev1.Pod, volumeName string) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == volumeName {
			return false
		}
	}
	return true
}

func isContainerVolumeMissing(container *corev1.Container, volumeName string) bool {
	for _, volume := range container.VolumeMounts {
		if volume.Name == volumeName {
			return false
		}
	}
	return true
}

func getIndexOfEnv(envs []corev1.EnvVar, name string) int {
	for i := range envs {
		if envs[i].Name == name {
			return i
		}
	}
	return -1
}

func getValueFromEnv(envVars []corev1.EnvVar, name string) (string, bool) {
	for _, env := range envVars {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

// setEnvVar function sets env var to the container if not exist already.
// value of concatValues should be set to true if the env var supports multiple values separated by a string.  It will only be appended if it does not exist
// If it is set to false, the original container's env var value has priority.
func setEnvVar(container *corev1.Container, envVarName string, envVarValue string, concatValues bool, separator string) {
	idx := getIndexOfEnv(container.Env, envVarName)
	if idx < 0 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envVarName,
			Value: envVarValue,
		})
		return
	}
	if concatValues {
		if !strings.Contains(separator+container.Env[idx].Value+separator, separator+envVarValue+separator) {
			container.Env[idx].Value = container.Env[idx].Value + separator + envVarValue
		}
	}
}

func setContainerEnvFromInst(container *corev1.Container, inst current.Instrumentation) {
	for _, env := range inst.Spec.Agent.Env {
		if idx := getIndexOfEnv(container.Env, env.Name); idx == -1 {
			container.Env = append(container.Env, env)
		}
	}
}

func validateContainerEnv(envs []corev1.EnvVar, envsToBeValidated ...string) error {
	for _, envToBeValidated := range envsToBeValidated {
		for _, containerEnv := range envs {
			if containerEnv.Name == envToBeValidated {
				if containerEnv.ValueFrom != nil {
					return fmt.Errorf("the container defines env var value via ValueFrom, envVar: %s", containerEnv.Name)
				}
				break
			}
		}
	}
	return nil
}

func addPodVolumeIfMissing(pod *corev1.Pod, volumeName string) {
	if isPodVolumeMissing(pod, volumeName) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
}

func addContainerVolumeIfMissing(container *corev1.Container, volumeName string, mountPath string) {
	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: mountPath,
		})
	}
}

func addContainer(isTargetInitContainer bool, targetContainerName string, pod *corev1.Pod, newContainer corev1.Container) {
	if isTargetInitContainer {
		index := getInitContainerIndex(pod, targetContainerName)
		pod.Spec.InitContainers = insertContainerBeforeIndex(pod.Spec.InitContainers, index, newContainer)
	} else {
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, newContainer)
	}
}

func decodeAttributes(str string, fieldSeparator string, valueSeparator string) map[string]string {
	labelAttributes := map[string]string{}
	for _, attr := range strings.Split(str, fieldSeparator) {
		attrParts := strings.SplitN(attr, valueSeparator, 2)
		if len(attrParts) != 2 {
			continue
		}
		attrKey, attrValue := attrParts[0], attrParts[1]
		labelAttributes[attrKey] = attrValue
	}
	return labelAttributes
}

func encodeAttributes(m map[string]string, fieldSeparator string, valueSeparator string) string {
	str := ""
	i := 0
	keys := make([]string, len(m))
	for key := range m {
		keys[i] = key
		i++
	}
	slices.Sort(keys)
	for i, key := range keys {
		if i > 0 {
			str += fieldSeparator
		}
		str += key + valueSeparator + m[key]
	}
	return str
}

func setPodAnnotationFromInstrumentationVersion(pod *corev1.Pod, inst current.Instrumentation) error {
	instName := types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}.String()
	instVersions := map[string]string{}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if v, ok := pod.Annotations[instrumentationVersionAnnotation]; ok {
		if err := json.Unmarshal([]byte(v), &instVersions); err != nil {
			return fmt.Errorf("failed to unmarshal instrumentation version annotation, skipping adding new instrumentation version to pod annotation > %w", err)
		}
	}
	instVersions[instName] = fmt.Sprintf("%s/%d", inst.UID, inst.Generation)
	instVersionBytes, err := json.Marshal(instVersions)
	if err != nil {
		return fmt.Errorf("failed to marshal instrumentation version annotation > %w", err)
	}
	pod.Annotations[instrumentationVersionAnnotation] = string(instVersionBytes)
	return nil
}

func setAgentConfigMap(pod *corev1.Pod, container *corev1.Container, configMapName string, volumeName string, mountPath string) {
	if isPodVolumeMissing(pod, volumeName) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		})
	}

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: mountPath,
		})
	}
}

func setContainerEnvLicenseKey(container *corev1.Container, licenseKeySecretName string) {
	if idx := getIndexOfEnv(container.Env, EnvNewRelicLicenseKey); idx == -1 {
		optional := true
		container.Env = append(container.Env, corev1.EnvVar{
			Name: EnvNewRelicLicenseKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: licenseKeySecretName},
					Key:                  LicenseKey,
					Optional:             &optional,
				},
			},
		})
	}
}

func setContainerEnvInjectionDefaults(container *corev1.Container) {
	if idx := getIndexOfEnv(container.Env, EnvNewRelicLabels); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicLabels,
			Value: "operator:auto-injection",
		})
	} else {
		labelAttributes := decodeAttributes(container.Env[idx].Value, ";", ":")
		labelAttributes["operator"] = "auto-injection"
		container.Env[idx].Value = encodeAttributes(labelAttributes, ";", ":")
	}
	if idx := getIndexOfEnv(container.Env, EnvNewRelicK8sOperatorEnabled); idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicK8sOperatorEnabled,
			Value: "true",
		})
	}
}

func insertContainerBeforeIndex(containers []corev1.Container, index int, newContainer corev1.Container) []corev1.Container {
	initContainers := make([]corev1.Container, len(containers)+1)
	copy(initContainers, containers[0:index])
	initContainers[index] = newContainer
	copy(initContainers[index+1:], containers[index:])
	return initContainers
}

func generateContainerName(namePrefix string) string {
	if len(namePrefix) > maxContainerNameLength {
		return strings.TrimRight(namePrefix[:maxContainerNameLength-hashLength-1], "-") + "-" + fmt.Sprintf("%x", sha256.Sum256([]byte(namePrefix)))[:hashLength]
	}
	return namePrefix
}
