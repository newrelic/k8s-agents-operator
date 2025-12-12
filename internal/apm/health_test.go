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

func TestHealthInjector_Inject(t *testing.T) {
	restartAlways := corev1.ContainerRestartPolicyAlways
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		inst           current.Instrumentation
		expectedPod    corev1.Pod
		expectedErrStr string
		healthPort     int
		healthPath     string
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
			inst: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "not-this"}}},
		},
		{
			name: "a container, instrumentation healthAgent with only env",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
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
						Name:  "nri-health--test",
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/nri-health--test",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///nri-health--test"},
							{Name: envHealthListenPort, Value: "6194"},
							{Name: "test", Value: "test"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/nri-health--test",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///nri-health--test"},
							{Name: envAgentControlEnabled, Value: "true"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "nri-health--test",
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
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
						Image: "health",
						Env:   []corev1.EnvVar{{Name: "test", Value: "test"}},
					},
				},
			},
			healthPath: "file:///test/health/path",
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/apm-health": "true",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:  "nri-health--test",
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/test",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///test/health/path"},
							{Name: envHealthListenPort, Value: "6194"},
							{Name: "test", Value: "test"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/test",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///test/health/path"},
							{Name: envAgentControlEnabled, Value: "true"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "nri-health--test",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation with healthAgent",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
						Image: "health",
						Env:   []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"}},
					},
				},
			},
			healthPath: "file:///health/this",
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/apm-health": "true",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:  "nri-health--test",
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/health",
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
							Name:      "nri-health--test",
							MountPath: "/health",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
							{Name: envAgentControlEnabled, Value: "true"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "nri-health--test",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, ensure no dup env name/value in sidecar",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
							{Name: envHealthListenPort, Value: "6194"},
						},
					},
				},
			},
			healthPath: "file:///health/this",
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"newrelic.com/apm-health": "true"}},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:  "nri-health--test",
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/health",
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
							Name:      "nri-health--test",
							MountPath: "/health",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///health/this"},
							{Name: envAgentControlEnabled, Value: "true"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "nri-health--test",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, missing health file location, uses default",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
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
						Name:  "nri-health--test",
						Image: "health",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/nri-health--test",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///nri-health--test"},
							{Name: envHealthListenPort, Value: "6194"},
						},
						RestartPolicy: &restartAlways,
						Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
					}},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/nri-health--test",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///nri-health--test"},
							{Name: envAgentControlEnabled, Value: "true"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "nri-health--test",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, invalid file path",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "/a/b"},
						},
					},
				},
			},
			healthPath:     "/a/b",
			expectedErrStr: "invalid env value \"/a/b\" for \"NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION\" > invalid health path \"/a/b\", must be file URI",
		},
		{
			name: "a container, instrumentation, (blank) health delivery location, uses default",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
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
					InitContainers: []corev1.Container{
						{
							Name:  "nri-health--test",
							Image: "health",
							VolumeMounts: []corev1.VolumeMount{{
								Name:      "nri-health--test",
								MountPath: "/nri-health--test",
							}},
							Env: []corev1.EnvVar{
								{Name: envAgentControlHealthDeliveryLocation, Value: "file:///nri-health--test"},
								{Name: envHealthListenPort, Value: "6194"},
							},
							RestartPolicy: &restartAlways,
							Ports:         []corev1.ContainerPort{{ContainerPort: 6194}},
						},
					},
					Containers: []corev1.Container{{
						Name: "test",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nri-health--test",
							MountPath: "/nri-health--test",
						}},
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///nri-health--test"},
							{Name: envAgentControlEnabled, Value: "true"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "nri-health--test",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					}},
				}},
		},
		{
			name: "a container, instrumentation, invalid (root) health path",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					HealthAgent: current.HealthAgent{
						Image: "health",
						Env: []corev1.EnvVar{
							{Name: envAgentControlHealthDeliveryLocation, Value: "file:///file.yml"},
						},
					},
				},
			},
			healthPath:     "file:///file.yml",
			expectedErrStr: "invalid env value \"file:///file.yml\" for \"NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION\" > invalid mount path \"file:///file.yml\", cannot have a file extension",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := NewHealthInjector()
			containerName := ""
			if len(test.pod.Spec.Containers) > 0 {
				containerName = test.pod.Spec.Containers[0].Name
			}
			actualPod, err := i.Inject(ctx, test.inst, test.ns, test.pod, containerName, test.healthPort, test.healthPath)
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
