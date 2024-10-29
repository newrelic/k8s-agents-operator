package apm

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
)

func TestPhpInjector_Language(t *testing.T) {
	require.Equal(t, "php", (&PhpInjector{acceptVersion: acceptVersion("php")}).Language())
}

func TestPhpInjector_Inject(t *testing.T) {
	vtrue := true
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		inst           v1alpha2.Instrumentation
		expectedPod    corev1.Pod
		expectedErrStr string
	}{
		{
			name: "nothing",
		},
		{
			name: "a container, no instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
		},
		{
			name: "a container, wrong instrumentation (not the correct lang)",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "not-this"}}},
		},
		{
			name: "a container, instrumentation with blank licenseKeySecret",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedErrStr: "licenseKeySecret must not be blank",
			inst:           v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "php-8.3"}}},
		},
		{
			name: "a container, instrumentation",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"instrumentation.newrelic.com/php-version": "8.3"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "test"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"instrumentation.newrelic.com/php-version": "8.3"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "PHP_INI_SCAN_DIR", Value: "/newrelic-instrumentation/php-agent/ini"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					InitContainers: []corev1.Container{{
						Name: "newrelic-instrumentation-php",
						Env: []corev1.EnvVar{
							{Name: "PHP_INI_SCAN_DIR", Value: "/newrelic-instrumentation/php-agent/ini"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						Command:      []string{"/bin/sh"},
						Args:         []string{"-c", "cp -a /instrumentation/. /newrelic-instrumentation/ && /newrelic-instrumentation/k8s-php-install.sh 20230831 && /newrelic-instrumentation/nr_env_to_ini.sh"},
						VolumeMounts: []corev1.VolumeMount{{Name: "newrelic-instrumentation", MountPath: "/newrelic-instrumentation"}},
					}},
					Volumes: []corev1.Volume{{Name: "newrelic-instrumentation", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "php-8.3"}, LicenseKeySecret: "newrelic-key-secret"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &PhpInjector{acceptVersion: acceptVersion("php-8.3")}
			actualPod, err := i.Inject(ctx, test.inst, test.ns, test.pod)
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
