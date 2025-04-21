package util

import corev1 "k8s.io/api/core/v1"

func SetPodLabel(pod *corev1.Pod, key, val string) {
	labels := pod.Labels
	if labels == nil {
		pod.ObjectMeta.Labels = make(map[string]string)
	}
	pod.ObjectMeta.Labels[key] = val
}

func GetContainerByNameFromPod(pod *corev1.Pod, containerName string) (container *corev1.Container, isInitContainer bool) {
	for idx, podContainer := range pod.Spec.Containers {
		if podContainer.Name == containerName {
			return &pod.Spec.Containers[idx], false
		}
	}
	for idx, podContainer := range pod.Spec.InitContainers {
		if podContainer.Name == containerName {
			return &pod.Spec.InitContainers[idx], true
		}
	}
	return nil, false
}
