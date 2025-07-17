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

func TestPythonInjector_Inject(t *testing.T) {
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
				{Name: "test", Env: []corev1.EnvVar{{Name: envPythonPath, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: PYTHONPATH",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION already set using ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python"}, LicenseKeySecret: "VALID", HealthAgent: current.HealthAgent{Image: "health"}}}},
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
							{Name: "PYTHONPATH", Value: "/nri-python--test"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--test", MountPath: "/nri-python--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-python--test",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-python--test/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--test", MountPath: "/nri-python--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-python--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation, with existing env PYTHONPATH",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "test",
					Env: []corev1.EnvVar{
						{Name: "PYTHONPATH", Value: "fakepath"},
					},
				},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "PYTHONPATH", Value: "fakepath:/nri-python--test"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--test", MountPath: "/nri-python--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-python--test",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-python--test/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--test", MountPath: "/nri-python--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-python--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation, with existing env PYTHONPATH",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-thing",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name: "alpine1",
						},
						{
							Name: "init-python",
						},
						{
							Name: "any-python1",
						},
					},
					Containers: []corev1.Container{
						{
							Name: "alpine2",
						},
						{
							Name: "any-python2",
						},
						{
							Name: "python",
						},
					},
				},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/a":"/0","/b":"/0","/c":"/0"}`,
					},
					Name: "pod-thing",
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "nri-python--init-python", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "nri-python--any-python1", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "nri-python--any-python2", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "nri-python--python", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
					InitContainers: []corev1.Container{
						{
							Name: "alpine1",
						},
						{
							Name:         "nri-python--init-python",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-python--init-python/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--init-python", MountPath: "/nri-python--init-python"}},
						},
						{
							Name: "init-python",
							Env: []corev1.EnvVar{

								{Name: "PYTHONPATH", Value: "/nri-python--init-python"},
								{Name: "NEW_RELIC_TARGET_INIT", Value: "C"},
								{Name: "NEW_RELIC_APP_NAME", Value: "pod-thing"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--init-python", MountPath: "/nri-python--init-python"}},
						},
						{
							Name:         "nri-python--any-python1",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-python--any-python1/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--any-python1", MountPath: "/nri-python--any-python1"}},
						},
						{
							Name: "any-python1",
							Env: []corev1.EnvVar{

								{Name: "PYTHONPATH", Value: "/nri-python--any-python1"},
								{Name: "NEW_RELIC_TARGET_ANY", Value: "A"},
								{Name: "NEW_RELIC_APP_NAME", Value: "pod-thing"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--any-python1", MountPath: "/nri-python--any-python1"}},
						},
						{
							Name:         "nri-python--any-python2",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-python--any-python2/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--any-python2", MountPath: "/nri-python--any-python2"}},
						},
						{
							Name:         "nri-python--python",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-python--python/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--python", MountPath: "/nri-python--python"}},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "alpine2",
						},
						{
							Name: "any-python2",
							Env: []corev1.EnvVar{

								{Name: "PYTHONPATH", Value: "/nri-python--any-python2"},
								{Name: "NEW_RELIC_TARGET_ANY", Value: "A"},
								{Name: "NEW_RELIC_APP_NAME", Value: "pod-thing"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--any-python2", MountPath: "/nri-python--any-python2"}},
						},
						{
							Name: "python",
							Env: []corev1.EnvVar{

								{Name: "PYTHONPATH", Value: "/nri-python--python"},
								{Name: "NEW_RELIC_TARGET_CONTAINER", Value: "B"},
								{Name: "NEW_RELIC_APP_NAME", Value: "pod-thing"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-python--python", MountPath: "/nri-python--python"}},
						},
					},
				},
			},
			mutations: []mutation{
				{
					containerName: "init-python",
					instrumentation: current.Instrumentation{
						ObjectMeta: metav1.ObjectMeta{Name: "a"},
						Spec: current.InstrumentationSpec{
							Agent: current.Agent{
								Language: "python",
								Env: []corev1.EnvVar{
									{Name: "NEW_RELIC_TARGET_INIT", Value: "C"},
								},
							},
							LicenseKeySecret: "newrelic-key-secret",
						},
					},
				},
				{
					containerName: "any-python1",
					instrumentation: current.Instrumentation{
						ObjectMeta: metav1.ObjectMeta{Name: "b"},
						Spec: current.InstrumentationSpec{
							Agent: current.Agent{
								Language: "python",
								Env: []corev1.EnvVar{
									{Name: "NEW_RELIC_TARGET_ANY", Value: "A"},
								},
							},
							LicenseKeySecret: "newrelic-key-secret",
						},
					},
				},
				{
					containerName: "any-python2",
					instrumentation: current.Instrumentation{
						ObjectMeta: metav1.ObjectMeta{Name: "b"},
						Spec: current.InstrumentationSpec{
							Agent: current.Agent{
								Language: "python",
								Env: []corev1.EnvVar{
									{Name: "NEW_RELIC_TARGET_ANY", Value: "A"},
								},
							},
							LicenseKeySecret: "newrelic-key-secret",
						},
					},
				},
				{
					containerName: "python",
					instrumentation: current.Instrumentation{
						ObjectMeta: metav1.ObjectMeta{Name: "c"},
						Spec: current.InstrumentationSpec{
							Agent: current.Agent{
								Language: "python",
								Env: []corev1.EnvVar{
									{Name: "NEW_RELIC_TARGET_CONTAINER", Value: "B"},
								},
							},
							LicenseKeySecret: "newrelic-key-secret",
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &PythonInjector{baseInjector{lang: "python"}}
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
