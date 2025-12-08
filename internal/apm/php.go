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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/util"
)

const (
	envIniScanDirKey = "PHP_INI_SCAN_DIR"
)

var _ ContainerInjector = (*PhpInjector)(nil)

func init() {
	for _, v := range phpAcceptVersions {
		DefaultInjectorRegistry.MustRegister(&PhpInjector{baseInjector{lang: string(v)}})
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

type PhpInjector struct {
	baseInjector
}

//nolint:nestif
func (i *PhpInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}
	apiNum, ok := phpApiMap[acceptVersion(i.Language())]
	if !ok {
		return pod, errors.New("invalid php version")
	}

	initContainerName := generateContainerName("nri-php--" + containerName)
	volumeName := initContainerName
	mountPath := "/" + initContainerName
	envIniScanDirVal := mountPath + "/php-agent/ini"

	if err := validateContainerEnv(container.Env, envIniScanDirKey); err != nil {
		return corev1.Pod{}, err
	}
	// set a blank value, so that php will scan the config dir that was configured during compilation with --with-config-file-scan-dir
	// do this first so we can override the behavior later.  We only set it if it was already blank or the key was not defined
	if val, ok := getValueFromEnv(container.Env, envIniScanDirKey); !ok || val == "" {
		setEnvVar(container, envIniScanDirKey, "", true, ":")
	}
	setEnvVar(container, envIniScanDirKey, envIniScanDirVal, true, ":")
	setContainerEnvFromInst(container, inst)

	addPodVolumeIfMissing(&pod, volumeName)
	addContainerVolumeIfMissing(container, volumeName, mountPath)

	if err := i.setContainerEnvAppName(ctx, &ns, &pod, container); err != nil {
		return corev1.Pod{}, err
	}
	setContainerEnvInjectionDefaults(container)

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(&pod, initContainerName) {
		copyOfContainerEnv := make([]corev1.EnvVar, 0, len(container.Env))
		for _, entry := range container.Env {
			if strings.HasPrefix(entry.Name, "NEWRELIC_") || strings.HasPrefix(entry.Name, "NEW_RELIC_") {
				copyOfContainerEnv = append(copyOfContainerEnv, *entry.DeepCopy())
			}
		}
		commands := []string{
			"cp -a /instrumentation/. " + mountPath + "/",

			// fix up the paths
			"sed -i 's@/newrelic-instrumentation@" + mountPath + "@g' " + mountPath + "/php-agent/ini/newrelic.ini",
			"sed -i 's@/newrelic-instrumentation@" + mountPath + "@g' " + mountPath + "/k8s-php-install.sh",
			"sed -i 's@/newrelic-instrumentation@" + mountPath + "@g' " + mountPath + "/nr_env_to_ini.sh",

			// setup the correct libs and bins
			mountPath + "/k8s-php-install.sh " + apiNum,

			// copy env based on mapping to newrelic.ini
			mountPath + "/nr_env_to_ini.sh",
		}
		newContainer := corev1.Container{
			Name:            initContainerName,
			Image:           inst.Spec.Agent.Image,
			ImagePullPolicy: inst.Spec.Agent.ImagePullPolicy,
			Command:         []string{"/bin/sh"},
			Args: []string{
				"-c", strings.Join(commands, " && "),
			},
			Env: copyOfContainerEnv,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: mountPath,
			}},
			Resources:       *inst.Spec.Agent.Resources.DeepCopy(),
			SecurityContext: inst.Spec.Agent.SecurityContext.DeepCopy(),
		}
		setContainerEnvLicenseKey(&newContainer, inst.Spec.LicenseKeySecret)
		addContainer(isTargetInitContainer, containerName, &pod, newContainer)
	}

	if err := setPodAnnotationFromInstrumentationVersion(&pod, inst); err != nil {
		return corev1.Pod{}, err
	}
	return pod, nil
}
