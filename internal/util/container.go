package util

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func ParseContainerImage(image string) map[string]string {
	parts := strings.SplitN(image, "/", 3)
	registry, namespace, name := "docker.io", "library", parts[0]
	switch len(parts) {
	case 2:
		if parts[0] == "docker.io" || parts[0] == "localhost" {
			registry, namespace, name = parts[0], "library", parts[1]
		} else {
			registry, namespace, name = "docker.io", parts[0], parts[1]
		}
	case 3:
		registry, namespace, name = parts[0], parts[1], parts[2]
	}
	m := map[string]string{"registry": registry, "namespace": namespace}
	if nameParts := strings.SplitN(name, "@", 2); len(nameParts) == 2 {
		name, m["digest"] = nameParts[0], nameParts[1]
	}
	if nameParts := strings.SplitN(name, ":", 2); len(nameParts) == 2 {
		name, m["tag"] = nameParts[0], nameParts[1]
	}
	m["name"] = name
	if m["tag"] == "" && m["digest"] == "" {
		m["tag"] = "latest"
	}
	return m
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
