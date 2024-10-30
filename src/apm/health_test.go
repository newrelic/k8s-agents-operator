package apm

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

func TestHealthInjector_Language(t *testing.T) {
	require.Equal(t, "health", (&HealthInjector{}).Language())
}

func TestHealthInjector_Inject(t *testing.T) {
	restartAlways := corev1.ContainerRestartPolicyAlways
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
			inst:           v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "health"}}},
		},
		{
			name: "a container, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{
				Agent: v1alpha2.Agent{
					Language: "health",
					Env:      []corev1.EnvVar{{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_FILE", Value: "/health/this"}},
				},
				LicenseKeySecret: "newrelic-key-secret"},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{
					Name: "newrelic-apm-health",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "newrelic-apm-health",
						MountPath: "/health",
					}},
					Env: []corev1.EnvVar{
						{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_FILE", Value: "/health/this"},
						{Name: "NEW_RELIC_SIDECAR_LISTEN_PORT", Value: "6194"},
						{Name: "NEW_RELIC_SIDECAR_TIMEOUT_DURATION", Value: "1s"},
					},
					RestartPolicy: &restartAlways,
					Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
				}},
				Containers: []corev1.Container{{
					Name: "test",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "newrelic-apm-health",
						MountPath: "/health",
					}},
					Env: []corev1.EnvVar{
						{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_FILE", Value: "/health/this"},
					},
				}},
				Volumes: []corev1.Volume{{
					Name:         "newrelic-apm-health",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				}},
			}},
		},
		{
			name: "a container, instrumentation, missing health file",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{
				Agent: v1alpha2.Agent{
					Language: "health",
				},
				LicenseKeySecret: "newrelic-key-secret"},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_FILE\" > invalid mount path \"\" from value \"\", cannot be blank",
		},
		{
			name: "a container, instrumentation, invalid timeout",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{
				Agent: v1alpha2.Agent{
					Language: "health",
					Env: []corev1.EnvVar{
						{Name: envHealthFleetControlFile, Value: "/a/b"},
						{Name: envHealthTimeout, Value: "not a duration"},
					},
				},
				LicenseKeySecret: "newrelic-key-secret"},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"not a duration\" for \"NEW_RELIC_SIDECAR_TIMEOUT_DURATION\" > invalid timeout \"not a duration\" > time: invalid duration \"not a duration\"",
		},
		{
			name: "a container, instrumentation, invalid port",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{
				Agent: v1alpha2.Agent{
					Language: "health",
					Env: []corev1.EnvVar{
						{Name: envHealthFleetControlFile, Value: "/a/b"},
						{Name: envHealthListenPort, Value: "not a port"},
					},
				},
				LicenseKeySecret: "newrelic-key-secret"},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"not a port\" for \"NEW_RELIC_SIDECAR_LISTEN_PORT\" > invalid health listen port \"not a port\" > strconv.Atoi: parsing \"not a port\": invalid syntax",
		},
		{
			name: "a container, instrumentation, invalid (blank) health file",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{
				Agent: v1alpha2.Agent{
					Language: "health",
					Env: []corev1.EnvVar{
						{Name: envHealthFleetControlFile, Value: ""},
					},
				},
				LicenseKeySecret: "newrelic-key-secret"},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_FILE\" > invalid mount path \"\" from value \"\", cannot be blank",
		},
		{
			name: "a container, instrumentation, invalid (root) health path",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{
				Agent: v1alpha2.Agent{
					Language: "health",
					Env: []corev1.EnvVar{
						{Name: envHealthFleetControlFile, Value: "/file"},
					},
				},
				LicenseKeySecret: "newrelic-key-secret"},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"/file\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_FILE\" > invalid mount path \"/\" from value \"/file\", cannot be root",
		},
		{
			name: "a container, instrumentation, invalid (root) health file",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{
				Agent: v1alpha2.Agent{
					Language: "health",
					Env: []corev1.EnvVar{
						{Name: envHealthFleetControlFile, Value: "/just-a-directory/"},
					},
				},
				LicenseKeySecret: "newrelic-key-secret"},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"/just-a-directory/\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_FILE\" > invalid mount file \"\" from value \"/just-a-directory/\", cannot be blank",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &HealthInjector{}
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
