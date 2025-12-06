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

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/util"
)

const (
	envRubyOpt = "RUBYOPT"
)

var _ ContainerInjector = (*RubyInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&RubyInjector{baseInjector{lang: "ruby"}})
}

type RubyInjector struct {
	baseInjector
}

func (i *RubyInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}

	initContainerName := generateContainerName("nri-ruby--" + containerName)
	volumeName := initContainerName
	mountPath := "/" + volumeName
	rubyOptRequire := "-r " + mountPath + "/lib/boot/strap"

	if err := validateContainerEnv(container.Env, envRubyOpt); err != nil {
		return corev1.Pod{}, err
	}
	setEnvVar(container, envRubyOpt, rubyOptRequire, true, " ")
	setContainerEnvFromInst(container, inst)

	addPodVolumeIfMissing(&pod, volumeName)
	addContainerVolumeIfMissing(container, volumeName, mountPath)

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(&pod, initContainerName) {
		newContainer := corev1.Container{
			Name:            initContainerName,
			Image:           inst.Spec.Agent.Image,
			ImagePullPolicy: inst.Spec.Agent.ImagePullPolicy,
			Command:         []string{"cp", "-a", "/instrumentation/.", mountPath + "/"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: mountPath,
			}},
			Resources:       *inst.Spec.Agent.Resources.DeepCopy(),
			SecurityContext: inst.Spec.Agent.SecurityContext.DeepCopy(),
		}
		addContainer(isTargetInitContainer, containerName, &pod, newContainer)

		// re get container, it's address in memory likely changed, since appending can allocate a new slice
		container, _ = util.GetContainerByNameFromPod(&pod, containerName)
	}

	if err := i.setContainerEnvAppName(ctx, &ns, &pod, container); err != nil {
		return corev1.Pod{}, err
	}
	setContainerEnvInjectionDefaults(container)
	setContainerEnvLicenseKey(container, inst.Spec.LicenseKeySecret)
	if err := setPodAnnotationFromInstrumentationVersion(&pod, inst); err != nil {
		return corev1.Pod{}, err
	}
	return pod, nil
}
