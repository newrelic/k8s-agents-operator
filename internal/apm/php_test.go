package apm

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

func TestPhpInjector_Inject(t *testing.T) {
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
				{Name: "test", Env: []corev1.EnvVar{{Name: envIniScanDirKey, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: PHP_INI_SCAN_DIR",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-8.3"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION already set using ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-8.3"}, LicenseKeySecret: "VALID", HealthAgent: current.HealthAgent{Image: "health"}}}},
			},
		},
		{
			name: "a container, instrumentation",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"instrumentation.newrelic.com/php-version": "8.3"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "a", Value: "a"},
						},
					},
				}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"instrumentation.newrelic.com/php-version": "8.3",
						"newrelic.com/instrumentation-versions":    `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "a", Value: "a"},
							{Name: "PHP_INI_SCAN_DIR", Value: ":/nri-php--test/php-agent/ini"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-php--test", MountPath: "/nri-php--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name: "nri-php--test",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						Command: []string{"/bin/sh"},
						Args: []string{"-c", strings.Join([]string{
							"cp -a /instrumentation/. /nri-php--test/",
							"sed -i 's@/newrelic-instrumentation@/nri-php--test@g' /nri-php--test/php-agent/ini/newrelic.ini",
							"sed -i 's@/newrelic-instrumentation@/nri-php--test@g' /nri-php--test/k8s-php-install.sh",
							"sed -i 's@/newrelic-instrumentation@/nri-php--test@g' /nri-php--test/nr_env_to_ini.sh",
							"/nri-php--test/k8s-php-install.sh 20230831",
							"/nri-php--test/nr_env_to_ini.sh",
						}, " && ")},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-php--test", MountPath: "/nri-php--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-php--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-8.3"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation, with existing env PHP_INI_SCAN_DIR",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"instrumentation.newrelic.com/php-version": "8.3"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "a", Value: "a"},
							{Name: "PHP_INI_SCAN_DIR", Value: "fakepath"},
						},
					},
				}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"instrumentation.newrelic.com/php-version": "8.3",
						"newrelic.com/instrumentation-versions":    `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "a", Value: "a"},
							{Name: "PHP_INI_SCAN_DIR", Value: "fakepath:/nri-php--test/php-agent/ini"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-php--test", MountPath: "/nri-php--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name: "nri-php--test",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						Command: []string{"/bin/sh"},
						Args: []string{"-c", strings.Join([]string{
							"cp -a /instrumentation/. /nri-php--test/",
							"sed -i 's@/newrelic-instrumentation@/nri-php--test@g' /nri-php--test/php-agent/ini/newrelic.ini",
							"sed -i 's@/newrelic-instrumentation@/nri-php--test@g' /nri-php--test/k8s-php-install.sh",
							"sed -i 's@/newrelic-instrumentation@/nri-php--test@g' /nri-php--test/nr_env_to_ini.sh",
							"/nri-php--test/k8s-php-install.sh 20230831",
							"/nri-php--test/nr_env_to_ini.sh",
						}, " && ")},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-php--test", MountPath: "/nri-php--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-php--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-8.3"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &PhpInjector{baseInjector{lang: "php-8.3"}}
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
