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

	"github.com/newrelic/k8s-agents-operator/api/current"
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

const (
	php72 acceptVersion = "php-7.2"
	php73 acceptVersion = "php-7.3"
	php74 acceptVersion = "php-7.4"
	php80 acceptVersion = "php-8.0"
	php81 acceptVersion = "php-8.1"
	php82 acceptVersion = "php-8.2"
	php83 acceptVersion = "php-8.3"
	php84 acceptVersion = "php-8.4"
)

var phpApiMap = map[acceptVersion]string{
	php72: "20170718",
	php73: "20180731",
	php74: "20190902",
	php80: "20200930",
	php81: "20210902",
	php82: "20220829",
	php83: "20230831",
	php84: "20240924",
}

var phpAcceptVersions = []acceptVersion{php72, php73, php74, php80, php81, php82, php83, php84}

type acceptVersion string

func (al acceptVersion) Language() string {
	return string(al)
}

func (al acceptVersion) acceptable(inst current.Instrumentation, pod corev1.Pod) bool {
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
func (i *PhpInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	firstContainer := 0

	apiNum, ok := phpApiMap[acceptVersion(i.Language())]
	if !ok {
		return pod, errors.New("invalid php version")
	}

	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[firstContainer]

	if err := validateContainerEnv(container.Env, envIniScanDirKey); err != nil {
		return pod, err
	}
	// set a blank value, so that php will scan the config dir that was configured during compilation with --with-config-file-scan-dir
	// do this first so we can override the behavior later.  We only set it if it was already blank or the key was not defined
	if val, ok := getValueFromEnv(container.Env, envIniScanDirKey); !ok || val == "" {
		setEnvVar(container, envIniScanDirKey, "", true, ":")
	}
	setEnvVar(container, envIniScanDirKey, envIniScanDirVal, true, ":")
	setContainerEnvFromInst(container, inst)

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/newrelic-instrumentation",
		})
	}

	pod = i.injectNewrelicEnvConfig(ctx, pod, firstContainer)

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, phpInitContainerName) {
		if isPodVolumeMissing(pod, volumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		copyOfContainerEnv := make([]corev1.EnvVar, 0, len(container.Env))
		for _, entry := range container.Env {
			if strings.HasPrefix(entry.Name, "NEWRELIC_") || strings.HasPrefix(entry.Name, "NEW_RELIC_") {
				copyOfContainerEnv = append(copyOfContainerEnv, *entry.DeepCopy())
			}
		}
		initContainer := corev1.Container{
			Name:    phpInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"/bin/sh"},
			Args: []string{
				"-c", "cp -a /instrumentation/. /newrelic-instrumentation/ && /newrelic-instrumentation/k8s-php-install.sh " + apiNum + " && /newrelic-instrumentation/nr_env_to_ini.sh",
			},
			Env: copyOfContainerEnv,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
		}
		initContainer = i.injectNewrelicLicenseKeyIntoContainer(initContainer, inst.Spec.LicenseKeySecret)
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, initContainer)
	}

	pod = addAnnotationToPodFromInstrumentationVersion(ctx, pod, inst)

	var err error
	if pod, err = i.injectHealth(ctx, inst, ns, pod, -1, getInitContainerIndex(pod, phpInitContainerName)); err != nil {
		return pod, err
	}

	return pod, nil
}
