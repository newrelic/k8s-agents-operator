package apm

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/newrelic/k8s-agents-operator/api/v1beta1"
)

func TestHealthInjector_Inject(t *testing.T) {
	restartAlways := corev1.ContainerRestartPolicyAlways
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		inst           v1beta1.Instrumentation
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
			inst: v1beta1.Instrumentation{Spec: v1beta1.InstrumentationSpec{Agent: v1beta1.Agent{Language: "not-this"}}},
		},
		{
			name: "a container, instrumentation healthAgent with only env",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env:   []corev1.EnvVar{{Name: "test", Value: "test"}},
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
						Name:  HealthSidecarContainerName,
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/newrelic/apm/health",
						}},
						Env: []corev1.EnvVar{
							{Name: "test", Value: "test"},
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///newrelic/apm/health"},
							{Name: envHealthListenPort, Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/newrelic/apm/health",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///newrelic/apm/health"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         healthVolumeName,
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation healthAgent with path only defined in main container",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "test",
				Env: []corev1.EnvVar{
					{Name: envAgentControlHealthDeliveryLocation, Value: "file:///test/health/path"},
				},
			}}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env:   []corev1.EnvVar{{Name: "test", Value: "test"}},
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
						Name:  HealthSidecarContainerName,
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/test/health/path",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///test/health/path"},
							{Name: "test", Value: "test"},
							{Name: envHealthListenPort, Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/test/health/path",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///test/health/path"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         healthVolumeName,
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation with healthAgent",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env:   []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"}},
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
						Name:  HealthSidecarContainerName,
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
							{Name: envHealthListenPort, Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         healthVolumeName,
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, ensure no dup env name/value in sidecar",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
							{Name: envHealthListenPort, Value: "6194"},
						},
					},
				},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"newrelic.com/apm-health": "true"}},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:  HealthSidecarContainerName,
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
							{Name: envHealthListenPort, Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/health/this",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         healthVolumeName,
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, missing health file",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
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
						Name:  HealthSidecarContainerName,
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/newrelic/apm/health",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///newrelic/apm/health"},
							{Name: envHealthListenPort, Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/newrelic/apm/health",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///newrelic/apm/health"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         healthVolumeName,
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, invalid port",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///a/b"},
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
			name: "a container, instrumentation, invalid file path",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "/a/b"},
						},
					},
				},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"/a/b\" for \"NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION\" > invalid health path \"/a/b\", must be file URI",
		},
		{
			name: "a container, instrumentation, invalid (blank) health file",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: ""},
						},
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
						Name:  HealthSidecarContainerName,
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/newrelic/apm/health",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///newrelic/apm/health"},
							{Name: envHealthListenPort, Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      healthVolumeName,
							MountPath: "/newrelic/apm/health",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///newrelic/apm/health"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         healthVolumeName,
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, invalid (root) health path",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
					HealthAgent: v1beta1.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "/file.yml"},
						},
					},
				},
			},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "test",
				}},
			}},
			expectedErrStr: "invalid env value \"/file.yml\" for \"NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION\" > invalid mount path \"/file.yml\", cannot have a file extension",
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
