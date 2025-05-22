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
	rubyVolumeName        = "newrelic-instrumentation-ruby"
	rubyMountPath         = "/" + rubyVolumeName
	envRubyOpt            = "RUBYOPT"
	rubyOptRequire        = "-r " + rubyMountPath + "/lib/boot/strap"
	rubyInitContainerName = initContainerName + "-ruby"
)

var _ Injector = (*RubyInjector)(nil)
var _ ContainerInjector = (*RubyInjector)(nil)

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
	container, isTargetInitContainer := util.GetContainerByNameFromPod(&pod, containerName)
	if container == nil {
		return corev1.Pod{}, fmt.Errorf("container %q not found", containerName)
	}
	if err := validateContainerEnv(container.Env, envRubyOpt); err != nil {
		return corev1.Pod{}, err
	}
	setEnvVar(container, envRubyOpt, rubyOptRequire, true, " ")
	setContainerEnvFromInst(container, inst)

	if isContainerVolumeMissing(container, rubyVolumeName) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      rubyVolumeName,
			MountPath: rubyMountPath,
		})
	}

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, rubyInitContainerName) {
		pod = i.addContainer(pod, inst.Spec.Agent.Image, containerName, isTargetInitContainer)
	} else {
		if isTargetInitContainer {
			agentIndex := getInitContainerIndex(pod, rubyInitContainerName)
			targetIndex := getInitContainerIndex(pod, containerName)
			if targetIndex < agentIndex {
				// move our agent before the target, so that it runs before the target!
				var agentContainer corev1.Container
				pod.Spec.InitContainers, agentContainer = removeContainerByIndex(pod.Spec.InitContainers, agentIndex)
				pod.Spec.InitContainers = insertContainerBeforeIndex(pod.Spec.InitContainers, targetIndex, agentContainer)
			}
		}
	}

	if err := i.setContainerEnvAppName(ctx, &ns, &pod, container); err != nil {
		return corev1.Pod{}, err
	}
	setContainerEnvInjectionDefaults(container)
	setContainerEnvLicenseKey(container, inst.Spec.LicenseKeySecret)
	if err := setPodAnnotationFromInstrumentationVersion(&pod, inst); err != nil {
		return corev1.Pod{}, err
	}
	return i.injectHealthWithContainer(ctx, inst, ns, pod, container)
}

func (i *RubyInjector) addContainer(pod corev1.Pod, agentContainerImage string, targetContainerName string, isTargetInitContainer bool) corev1.Pod {
	if isPodVolumeMissing(pod, rubyVolumeName) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: rubyVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			}})
	}
	newContainer := corev1.Container{
		Name:    rubyInitContainerName,
		Image:   agentContainerImage,
		Command: []string{"cp", "-a", "/instrumentation/.", rubyMountPath + "/"},
		VolumeMounts: []corev1.VolumeMount{{
			Name:      rubyVolumeName,
			MountPath: rubyMountPath,
		}},
	}
	if isTargetInitContainer {
		index := getInitContainerIndex(pod, targetContainerName)
		pod.Spec.InitContainers = insertContainerBeforeIndex(pod.Spec.InitContainers, index, newContainer)
	} else {
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, newContainer)
	}
	return pod
}
