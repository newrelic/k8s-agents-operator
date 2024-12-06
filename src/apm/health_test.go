package apm

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

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
			name: "a container, blank instrumentation healthAgent",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "not-this"}}},
		},
		{
			name: "a container, instrumentation healthAgent with only env",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedErrStr: "invalid env value \"\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_PATH\" > invalid mount path \"\", cannot be blank",
			inst: v1alpha2.Instrumentation{
				Spec: v1alpha2.InstrumentationSpec{
					HealthAgent: v1alpha2.HealthAgent{
						Env: []corev1.EnvVar{{Name: "test", Value: "test"}},
					},
				},
			},
		},
		{
			name: "a container, instrumentation with healthAgent",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{
				Spec: v1alpha2.InstrumentationSpec{
					HealthAgent: v1alpha2.HealthAgent{
						Image: "health",
						Env:   []corev1.EnvVar{{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_PATH", Value: "/health/this"}},
					},
				},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/apm-health": "true",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name: "newrelic-apm-health-sidecar",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "newrelic-apm-health-volume",
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_PATH", Value: "/health/this"},
							{Name: "NEW_RELIC_SIDECAR_LISTEN_PORT", Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "newrelic-apm-health-volume",
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_PATH", Value: "/health/this"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "newrelic-apm-health-volume",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, ensure no dup env name/value in sidecar",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{
				Spec: v1alpha2.InstrumentationSpec{
					HealthAgent: v1alpha2.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_PATH", Value: "/health/this"},
							{Name: "NEW_RELIC_SIDECAR_LISTEN_PORT", Value: "6194"},
						},
					},
				},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"newrelic.com/apm-health": "true"}},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name: "newrelic-apm-health-sidecar",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "newrelic-apm-health-volume",
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_PATH", Value: "/health/this"},
							{Name: "NEW_RELIC_SIDECAR_LISTEN_PORT", Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "newrelic-apm-health-volume",
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_FLEET_CONTROL_HEALTH_PATH", Value: "/health/this"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "newrelic-apm-health-volume",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, missing health file",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{
				Spec: v1alpha2.InstrumentationSpec{
					HealthAgent: v1alpha2.HealthAgent{
						Image: "health",
					},
				},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_PATH\" > invalid mount path \"\", cannot be blank",
		},
		{
			name: "a container, instrumentation, invalid port",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{
				Spec: v1alpha2.InstrumentationSpec{
					HealthAgent: v1alpha2.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envHealthFleetControlFilepath, Value: "/a/b"},
							{Name: envHealthListenPort, Value: "not a port"},
						},
					},
				},
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
			inst: v1alpha2.Instrumentation{
				Spec: v1alpha2.InstrumentationSpec{
					HealthAgent: v1alpha2.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envHealthFleetControlFilepath, Value: ""},
						},
					},
				},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_PATH\" > invalid mount path \"\", cannot be blank",
		},
		{
			name: "a container, instrumentation, invalid (root) health path",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{
				Spec: v1alpha2.InstrumentationSpec{
					HealthAgent: v1alpha2.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envHealthFleetControlFilepath, Value: "/file.yml"},
						},
					},
				},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"/file.yml\" for \"NEW_RELIC_FLEET_CONTROL_HEALTH_PATH\" > invalid mount path \"/file.yml\", cannot have a file extension",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &baseInjector{}
			actualPod, err := i.injectHealth(ctx, test.inst, test.ns, test.pod)
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
