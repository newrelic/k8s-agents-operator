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
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		inst           current.Instrumentation
		expectedPod    corev1.Pod
		expectedErrStr string
		containerName  string
		useNewMethod   bool
	}{
		{
			name: "a container, instrumentation with env already set to ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envRubyOpt, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: RUBYOPT",
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "VALID"}},
		},
		{
			name: "a container, instrumentation with env NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION already set using ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION",
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "VALID", HealthAgent: current.HealthAgent{Image: "health"}}},
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
							{Name: "RUBYOPT", Value: "-r /newrelic-instrumentation/lib/boot/strap"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "newrelic-instrumentation-ruby",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					Volumes: []corev1.Volume{{Name: "newrelic-instrumentation", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				}},
			inst: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
		},
		{
			name:          "2 containers, inject the 2nd, instrumentation",
			containerName: "test-2",
			useNewMethod:  true,
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"}, {Name: "test-2"},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test"}, {
						Name: "test-2",
						Env: []corev1.EnvVar{
							{Name: "RUBYOPT", Value: "-r /newrelic-instrumentation/lib/boot/strap"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test-2"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "newrelic-instrumentation-ruby",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					Volumes: []corev1.Volume{{Name: "newrelic-instrumentation", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				}},
			inst: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
		},
		{
			name:          "1 container, 1 init container, inject the init container, instrumentation",
			containerName: "init-test",
			useNewMethod:  true,
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
							Name:         "newrelic-instrumentation-ruby",
							Command:      []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
							VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
						},
						{
							Name: "init-test",
							Env: []corev1.EnvVar{
								{Name: "RUBYOPT", Value: "-r /newrelic-instrumentation/lib/boot/strap"},
								{Name: "NEW_RELIC_APP_NAME", Value: "init-test"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
						},
					},
					Volumes: []corev1.Volume{{Name: "newrelic-instrumentation", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				}},
			inst: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
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
							{Name: "RUBYOPT", Value: "-r fakelib -r /newrelic-instrumentation/lib/boot/strap"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "newrelic-instrumentation-ruby",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/newrelic-instrumentation/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					Volumes: []corev1.Volume{{Name: "newrelic-instrumentation", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				}},
			inst: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby"}, LicenseKeySecret: "newrelic-key-secret"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			containerName := test.containerName
			if containerName == "" && len(test.pod.Spec.Containers) > 0 {
				containerName = test.pod.Spec.Containers[0].Name
			}
			ctx := context.Background()
			i := &RubyInjector{}
			// inject multiple times to assert that it's idempotent
			var err error
			var actualPod corev1.Pod
			testPod := test.pod
			for ic := 0; ic < 3; ic++ {
				if test.useNewMethod {
					actualPod, err = i.InjectContainer(ctx, test.inst, test.ns, test.pod, containerName)
				} else {
					actualPod, err = i.Inject(ctx, test.inst, test.ns, testPod)
				}
				if err != nil {
					break
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
		})
	}
}
