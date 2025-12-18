package apm

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

func TestJavaInjector_Inject(t *testing.T) {
	vtrue := true
	type mutation struct {
		containerName   string
		instrumentation current.Instrumentation
	}
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		expectedPod    corev1.Pod
		expectedErrStr string
		mutations      []mutation
	}{
		{
			name: "a container, instrumentation with env already set to ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envJavaToolsOptions, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: JAVA_TOOL_OPTIONS",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "JAVA_TOOL_OPTIONS", Value: "-javaagent:/nri-java--test/newrelic-agent.jar"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-java--test", MountPath: "/nri-java--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:    "nri-java--test",
						Command: []string{"/bin/sh"},
						Args: []string{
							"-c",
							"cp /newrelic-agent.jar /nri-java--test/newrelic-agent.jar && if test -d extensions; then cp -r extensions/. /nri-java--test/extensions/; fi",
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-java--test", MountPath: "/nri-java--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-java--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation, with existing env JAVA_TOOL_OPTIONS",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "test",
					Env: []corev1.EnvVar{
						{Name: "JAVA_TOOL_OPTIONS", Value: "-javaagent:someagent.jar"},
					},
				},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "JAVA_TOOL_OPTIONS", Value: "-javaagent:someagent.jar -javaagent:/nri-java--test/newrelic-agent.jar"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-java--test", MountPath: "/nri-java--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:    "nri-java--test",
						Command: []string{"/bin/sh"},
						Args: []string{
							"-c",
							"cp /newrelic-agent.jar /nri-java--test/newrelic-agent.jar && if test -d extensions; then cp -r extensions/. /nri-java--test/extensions/; fi",
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-java--test", MountPath: "/nri-java--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-java--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation with added new relic labels",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "test",
					Env: []corev1.EnvVar{
						{Name: "NEW_RELIC_LABELS", Value: "app:java-injected"},
					},
				},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_LABELS", Value: "app:java-injected;operator:auto-injection"},
							{Name: "JAVA_TOOL_OPTIONS", Value: "-javaagent:/nri-java--test/newrelic-agent.jar"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-java--test", MountPath: "/nri-java--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:    "nri-java--test",
						Command: []string{"/bin/sh"},
						Args: []string{
							"-c",
							"cp /newrelic-agent.jar /nri-java--test/newrelic-agent.jar && if test -d extensions; then cp -r extensions/. /nri-java--test/extensions/; fi",
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-java--test", MountPath: "/nri-java--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-java--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation, with an agent configmap",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test",
							Env: []corev1.EnvVar{
								{Name: "JAVA_TOOL_OPTIONS", Value: "-javaagent:/nri-java--test/newrelic-agent.jar"},
								{Name: "NEWRELIC_FILE", Value: "/nri-cfg--test/newrelic.yaml"},
								{Name: "NEW_RELIC_APP_NAME", Value: "test"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "nri-cfg--test", MountPath: "/nri-cfg--test"},
								{Name: "nri-java--test", MountPath: "/nri-java--test"},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    "nri-java--test",
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"cp /newrelic-agent.jar /nri-java--test/newrelic-agent.jar && if test -d extensions; then cp -r extensions/. /nri-java--test/extensions/; fi",
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-java--test", MountPath: "/nri-java--test"}},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "nri-cfg--test",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "my-java-apm-config",
									},
								},
							},
						}, {
							Name:         "nri-java--test",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java"}, LicenseKeySecret: "newrelic-key-secret", AgentConfigMap: "my-java-apm-config"}}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &JavaInjector{baseInjector{lang: "java"}}
			// inject multiple times to assert that it's idempotent. validate it's correct each time
			var err error
			var actualPod corev1.Pod
			testPod := test.pod

			for ic := 0; ic < 3; ic++ {
			mutLoop:
				for _, mut := range test.mutations {
					if !i.Accepts(mut.instrumentation, test.ns, testPod) {
						continue
					}
					containerName := mut.containerName
					if containerName == "" {
						containerName = testPod.Spec.Containers[0].Name
					}

					actualPod, err = i.InjectContainer(ctx, mut.instrumentation, test.ns, testPod, containerName)
					if err != nil {
						break mutLoop
					}
					testPod = actualPod
				}
				errStr := ""
				if err != nil {
					errStr = err.Error()
				}
				require.Equal(t, test.expectedErrStr, errStr)
				if diff := cmp.Diff(test.expectedPod, actualPod); diff != "" {
					assert.Fail(t, diff)
				}
			}
		})
	}
}
