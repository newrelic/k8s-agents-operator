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
	"errors"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

const (
	annotationPhpVersion = "instrumentation.newrelic.com/php-version"
	envIniScanDirKey     = "PHP_INI_SCAN_DIR"
	envIniScanDirVal     = "/newrelic-instrumentation/php-agent/ini"
	phpInitContainerName = initContainerName + "-php"
)

var _ Injector = (*PhpInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&PhpInjector{})
}

// Deprecated: phpApiMap is deprecated.  Do not use annotations.
var phpApiMap = map[string]string{
	"7.2": "20170718",
	"7.3": "20180731",
	"7.4": "20190902",
	"8.0": "20200930",
	"8.1": "20210902",
	"8.2": "20220829",
	"8.3": "20230831",
}

type PhpInjector struct {
	baseInjector
}

func (i *PhpInjector) Language() string {
	return "php"
}

func (i *PhpInjector) acceptable(inst v1alpha2.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

// Inject is used to inject the PHP agent.
// @todo: Currently it uses annotations, which should be removed.  This should either use a specific image for each php version or the k8s-agents-operator needs to add support for a language version
func (i *PhpInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	firstContainer := 0

	// exit early if we're missing mandatory annotations
	// Deprecated: phpVer is deprecated.  Do not use annotations.
	phpVer, ok := pod.Annotations[annotationPhpVersion]
	if !ok {
		return pod, errors.New("missing php version annotation")
	}

	// Deprecated: apiNum is deprecated.  Do not use annotations.
	apiNum, ok := phpApiMap[phpVer]
	if !ok {
		return pod, errors.New("invalid php version")
	}

	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	// inject PHP instrumentation spec env vars.
	for _, env := range inst.Spec.Agent.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	setEnvVar(container, envIniScanDirKey, envIniScanDirVal, true)

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/newrelic-instrumentation",
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, nodejsInitContainerName) {
		if isPodVolumeMissing(pod, volumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:    phpInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"/bin/sh"},
			Args: []string{
				"-c", "cp -a /instrumentation/. /newrelic-instrumentation/ && /newrelic-instrumentation/k8s-php-install.sh " + apiNum + " && /newrelic-instrumentation/nr_env_to_ini.sh",
			},
			Env: container.Env,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
		})
	}

	pod = i.injectNewrelicConfig(ctx, inst.Spec.Resource, ns, pod, firstContainer, inst.Spec.LicenseKeySecret)

	return pod, nil
}
