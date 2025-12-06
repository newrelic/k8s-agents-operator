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

func TestRubyInjector_Inject(t *testing.T) {
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
				{Name: "test", Env: []corev1.EnvVar{{Name: envRubyOpt, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: RUBYOPT",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "VALID"}}},
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
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "RUBYOPT", Value: "-r /nri-ruby--test/lib/boot/strap"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test", MountPath: "/nri-ruby--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-ruby--test",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--test/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test", MountPath: "/nri-ruby--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-ruby--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container with a long name, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test--container-with-a-really-long-name-that-might-just-be-over-descriptive"},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test--container-with-a-really-long-name-that-might-just-be-over-descriptive",
						Env: []corev1.EnvVar{
							{Name: "RUBYOPT", Value: "-r /nri-ruby--test--container-with-a-really-long-name-that-ee75c2a/lib/boot/strap"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test--container-with-a-really-long-name-that-might-just-be-over-descriptive"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test--container-with-a-really-long-name-that-ee75c2a", MountPath: "/nri-ruby--test--container-with-a-really-long-name-that-ee75c2a"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-ruby--test--container-with-a-really-long-name-that-ee75c2a",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--test--container-with-a-really-long-name-that-ee75c2a/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test--container-with-a-really-long-name-that-ee75c2a", MountPath: "/nri-ruby--test--container-with-a-really-long-name-that-ee75c2a"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-ruby--test--container-with-a-really-long-name-that-ee75c2a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "2 containers, inject the 2nd, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"}, {Name: "test-2"},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
					}, {
						Name: "test-2",
						Env: []corev1.EnvVar{
							{Name: "RUBYOPT", Value: "-r /nri-ruby--test-2/lib/boot/strap"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test-2"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test-2", MountPath: "/nri-ruby--test-2"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-ruby--test-2",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--test-2/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test-2", MountPath: "/nri-ruby--test-2"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-ruby--test-2", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{
					containerName:   "test-2",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
				},
			},
		},
		{
			name: "1 container, 1 init container, inject the init container, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{Name: "init-test"}},
				Containers:     []corev1.Container{{Name: "test"}},
			}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test"}},
					InitContainers: []corev1.Container{
						{
							Name:         "nri-ruby--init-test",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--init-test/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-test", MountPath: "/nri-ruby--init-test"}},
						},
						{
							Name: "init-test",
							Env: []corev1.EnvVar{
								{Name: "RUBYOPT", Value: "-r /nri-ruby--init-test/lib/boot/strap"},
								{Name: "NEW_RELIC_APP_NAME", Value: "init-test"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-test", MountPath: "/nri-ruby--init-test"}},
						},
					},
					Volumes: []corev1.Volume{{Name: "nri-ruby--init-test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{
					containerName:   "init-test",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
				},
			},
		},
		{
			name: "1 container, 3 init containers, inject 2nd init container, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{Name: "init-a"}, {Name: "init-b"}, {Name: "init-c"}},
				Containers:     []corev1.Container{{Name: "test"}},
			}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test"}},
					InitContainers: []corev1.Container{
						{
							Name: "init-a",
						},
						{
							Name:         "nri-ruby--init-b",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--init-b/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-b", MountPath: "/nri-ruby--init-b"}},
						},
						{
							Name: "init-b",
							Env: []corev1.EnvVar{
								{Name: "RUBYOPT", Value: "-r /nri-ruby--init-b/lib/boot/strap"},
								{Name: "NEW_RELIC_APP_NAME", Value: "init-b"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-b", MountPath: "/nri-ruby--init-b"}},
						},
						{
							Name: "init-c",
						},
					},
					Volumes: []corev1.Volume{{Name: "nri-ruby--init-b", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{
					containerName:   "init-b",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
				},
			},
		},
		{
			name: "1 container, 3 init containers, inject all the init containers in reverse order, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{Name: "init-a"}, {Name: "init-b"}, {Name: "init-c"}},
				Containers:     []corev1.Container{{Name: "test"}},
			}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test"}},
					InitContainers: []corev1.Container{
						{
							Name:         "nri-ruby--init-a",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--init-a/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-a", MountPath: "/nri-ruby--init-a"}},
						},
						{
							Name: "init-a",
							Env: []corev1.EnvVar{
								{Name: "RUBYOPT", Value: "-r /nri-ruby--init-a/lib/boot/strap"},
								{Name: "NEW_RELIC_APP_NAME", Value: "init-a"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-a", MountPath: "/nri-ruby--init-a"}},
						},
						{
							Name:         "nri-ruby--init-b",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--init-b/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-b", MountPath: "/nri-ruby--init-b"}},
						},
						{
							Name: "init-b",
							Env: []corev1.EnvVar{
								{Name: "RUBYOPT", Value: "-r /nri-ruby--init-b/lib/boot/strap"},
								{Name: "NEW_RELIC_APP_NAME", Value: "init-b"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-b", MountPath: "/nri-ruby--init-b"}},
						},
						{
							Name:         "nri-ruby--init-c",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--init-c/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-c", MountPath: "/nri-ruby--init-c"}},
						},
						{
							Name: "init-c",
							Env: []corev1.EnvVar{
								{Name: "RUBYOPT", Value: "-r /nri-ruby--init-c/lib/boot/strap"},
								{Name: "NEW_RELIC_APP_NAME", Value: "init-c"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--init-c", MountPath: "/nri-ruby--init-c"}},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "nri-ruby--init-c", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "nri-ruby--init-b", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "nri-ruby--init-a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
				},
			},
			mutations: []mutation{
				{
					containerName:   "init-c",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
				},
				{
					containerName:   "init-b",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
				},
				{
					containerName:   "init-a",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
				},
			},
		},
		{
			name: "a container, instrumentation, existing RUBYOPT",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "test",
					Env: []corev1.EnvVar{
						{
							Name:  "RUBYOPT",
							Value: "-r fakelib",
						},
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
							{Name: "RUBYOPT", Value: "-r fakelib -r /nri-ruby--test/lib/boot/strap"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test", MountPath: "/nri-ruby--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-ruby--test",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-ruby--test/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-ruby--test", MountPath: "/nri-ruby--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-ruby--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &RubyInjector{baseInjector{lang: "ruby"}}
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
