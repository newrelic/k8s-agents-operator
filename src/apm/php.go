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
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

const (
	envIniScanDirKey     = "PHP_INI_SCAN_DIR"
	envIniScanDirVal     = "/newrelic-instrumentation/php-agent/ini"
	phpInitContainerName = initContainerName + "-php"
)

var _ Injector = (*PhpInjector)(nil)

func init() {
	for _, v := range phpAcceptVersions {
		DefaultInjectorRegistry.MustRegister(&PhpInjector{acceptVersion: v})
	}
}

var phpApiMap = map[string]string{
	"7.2": "20170718",
	"7.3": "20180731",
	"7.4": "20190902",
	"8.0": "20200930",
	"8.1": "20210902",
	"8.2": "20220829",
	"8.3": "20230831",
}

var phpAcceptVersions = []acceptVersion{php72, php73, php74, php80, php81, php82, php83}

const (
	php72 acceptVersion = "php-7.2"
	php73 acceptVersion = "php-7.3"
	php74 acceptVersion = "php-7.4"
	php80 acceptVersion = "php-8.0"
	php81 acceptVersion = "php-8.1"
	php82 acceptVersion = "php-8.2"
	php83 acceptVersion = "php-8.3"
)

type acceptVersion string

func (al acceptVersion) Language() string {
	return string(al)
}

func (al acceptVersion) acceptable(inst v1alpha2.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != string(al) {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

type PhpInjector struct {
	baseInjector
	acceptVersion
}

// Inject is used to inject the PHP agent.
func (i *PhpInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	firstContainer := 0

	// <lang:php>-<version:[0-9].[0-9]>
	lang := strings.SplitN(i.Language(), "-", 2)
	if len(lang) != 2 {
		// should never happen
		return pod, errors.New("missing php version")
	}
	phpVer := lang[1]

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
