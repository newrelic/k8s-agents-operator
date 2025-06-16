package util

import (
	corev1 "k8s.io/api/core/v1"
)

func ParseContainerImage(image string) map[string]string {
	return map[string]string{
		"url": image,
	}
}

func EnvToMap(env []corev1.EnvVar) map[string]string {
	m := map[string]string{}
	for _, entry := range env {
		if entry.ValueFrom != nil {
			continue
		}
		m[entry.Name] = entry.Value
	}
	return m
}
