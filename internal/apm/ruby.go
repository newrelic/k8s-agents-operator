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
	envRubyOpt            = "RUBYOPT"
	rubyOptRequire        = "-r /newrelic-instrumentation/lib/boot/strap"
	rubyInitContainerName = initContainerName + "-ruby"
)

var _ Injector = (*RubyInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&RubyInjector{baseInjector{lang: "ruby"}})
}

type RubyInjector struct {
	baseInjector
}

func (i *RubyInjector) Inject(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	return i.InjectContainer(ctx, inst, ns, pod, pod.Spec.Containers[0].Name)
}

func (i *RubyInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	if containerName == "init-test" {
		for ci, cName := range pod.Spec.InitContainers {
			fmt.Printf("init container: %q, index: %d\n", cName.Name, ci)
		}
		for ci, cName := range pod.Spec.Containers {
			fmt.Printf("container: %q, index: %d\n", cName.Name, ci)
		}
	}
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}

	if err := validateContainerEnv(container.Env, envRubyOpt); err != nil {
		return corev1.Pod{}, err
	}
	setEnvVar(container, envRubyOpt, rubyOptRequire, true, " ")
	setContainerEnvFromInst(container, inst)

	if isContainerVolumeMissing(container, volumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: "/newrelic-instrumentation",
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, rubyInitContainerName) {
		if isPodVolumeMissing(pod, volumeName) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				}})
		}

		newContainer := corev1.Container{
			Name:    rubyInitContainerName,
			Image:   inst.Spec.Agent.Image,
			Command: []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volumeName,
				MountPath: "/newrelic-instrumentation",
			}},
		}
		if isTargetInitContainer {
			index := getInitContainerIndex(pod, containerName)
			initContainers := make([]corev1.Container, len(pod.Spec.InitContainers)+1)
			copy(initContainers, pod.Spec.InitContainers[0:index])
			initContainers[index] = newContainer
			copy(initContainers[index+1:], pod.Spec.InitContainers[index:])
			pod.Spec.InitContainers = initContainers
		} else {
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, newContainer)
		}
	}

	setContainerEnvInjectionDefaults(&pod, container)
	setContainerEnvLicenseKey(container, inst.Spec.LicenseKeySecret)
	if err := setPodAnnotationFromInstrumentationVersion(&pod, inst); err != nil {
		return corev1.Pod{}, err
	}
	return i.injectHealthWithContainer(ctx, inst, ns, pod, container)
}
