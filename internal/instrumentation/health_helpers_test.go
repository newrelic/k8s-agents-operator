package instrumentation

import (
	"context"
	"testing"

	"github.com/newrelic/k8s-agents-operator/internal/apm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetHealthUrlsFromPod(t *testing.T) {
	restartPolicyAlways := corev1.ContainerRestartPolicyAlways
	restartPolicyOnFailure := corev1.ContainerRestartPolicyOnFailure

	tests := []struct {
		name        string
		pod         *corev1.Pod
		expectedLen int
		wantErr     bool
		errContains string
	}{
		{
			name: "pod with health sidecar",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "nri-health--container",
							RestartPolicy: &restartPolicyAlways,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.1",
				},
			},
			expectedLen: 1,
			wantErr:     false,
		},
		{
			name: "pod without health sidecar",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "regular-init-container",
							RestartPolicy: &restartPolicyAlways,
						},
					},
				},
			},
			wantErr:     true,
			errContains: "health sidecar not found",
		},
		{
			name: "pod with sidecar but wrong restart policy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "nri-health--container",
							RestartPolicy: &restartPolicyOnFailure,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "health sidecar not found",
		},
		{
			name: "pod with health sidecar but no ports",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "nri-health--container",
							RestartPolicy: &restartPolicyAlways,
							Ports:         []corev1.ContainerPort{},
						},
					},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.1",
				},
			},
			wantErr:     true,
			errContains: "health sidecar missing exposed ports",
		},
		{
			name: "pod with health sidecar but too many ports",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "nri-health--container",
							RestartPolicy: &restartPolicyAlways,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080},
								{ContainerPort: 8081},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.1",
				},
			},
			wantErr:     true,
			errContains: "health sidecar has too many exposed ports",
		},
		{
			name: "pod with multiple health sidecars",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "nri-health--java",
							RestartPolicy: &restartPolicyAlways,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080},
							},
						},
						{
							Name:          "nri-health--python",
							RestartPolicy: &restartPolicyAlways,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8081},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.1",
				},
			},
			expectedLen: 2,
			wantErr:     false,
		},
		{
			name: "pod with init container but no restart policy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "nri-health--container",
							RestartPolicy: nil,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "health sidecar not found",
		},
		{
			name: "pod with short container name starting with nri-health",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "nri-health", // Only 10 chars, need 12+
							RestartPolicy: &restartPolicyAlways,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "health sidecar not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &HealthMonitor{}
			urls, err := m.getHealthUrlsFromPod(tt.pod)

			// Check error expectations
			if tt.wantErr && err == nil {
				t.Errorf("expected error containing %q, got nil", tt.errContains)
				return
			}
			if tt.wantErr && tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
				t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			// Check URL count
			if !tt.wantErr && len(urls) != tt.expectedLen {
				t.Errorf("expected %d URLs, got %d", tt.expectedLen, len(urls))
			}
		})
	}
}

func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "pod is running",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			expected: true,
		},
		{
			name: "pod is pending",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			expected: false,
		},
		{
			name: "pod is succeeded",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
			expected: false,
		},
		{
			name: "pod is failed",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
				},
			},
			expected: false,
		},
		{
			name: "pod is unknown",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodUnknown,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &HealthMonitor{}
			result := m.isPodReady(tt.pod)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsPodInstrumented(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "pod with health instrumented annotation set to true",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apm.HealthInstrumentedAnnotation: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with health instrumented annotation set to false",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apm.HealthInstrumentedAnnotation: "false",
					},
				},
			},
			expected: false,
		},
		{
			name: "pod without health instrumented annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "pod with nil annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			expected: false,
		},
		{
			name: "pod with invalid boolean value",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apm.HealthInstrumentedAnnotation: "invalid",
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with health instrumented annotation set to 1",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apm.HealthInstrumentedAnnotation: "1",
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with health instrumented annotation set to 0",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apm.HealthInstrumentedAnnotation: "0",
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with health instrumented annotation set to TRUE (uppercase)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apm.HealthInstrumentedAnnotation: "TRUE",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &HealthMonitor{}
			result := m.isPodInstrumented(tt.pod)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetPodMetrics(t *testing.T) {
	tests := []struct {
		name        string
		pods        map[string]*corev1.Pod
		expectedLen int
		expectedIDs []string
	}{
		{
			name:        "empty pods map",
			pods:        map[string]*corev1.Pod{},
			expectedLen: 0,
		},
		{
			name: "single pod",
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
					},
				},
			},
			expectedLen: 1,
			expectedIDs: []string{"default/pod1"},
		},
		{
			name: "multiple pods",
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
					},
				},
				"default/pod2": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
					},
				},
				"kube-system/pod3": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod3",
						Namespace: "kube-system",
					},
				},
			},
			expectedLen: 3,
			expectedIDs: []string{"default/pod1", "default/pod2", "kube-system/pod3"},
		},
		{
			name: "pod with special characters in name",
			pods: map[string]*corev1.Pod{
				"my-namespace/my-pod-123-abc": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-pod-123-abc",
						Namespace: "my-namespace",
					},
				},
			},
			expectedLen: 1,
			expectedIDs: []string{"my-namespace/my-pod-123-abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			m := &HealthMonitor{
				pods: tt.pods,
			}

			podMetrics := m.getPodMetrics(ctx)

			// Verify length
			if len(podMetrics) != tt.expectedLen {
				t.Errorf("expected %d pod metrics, got %d", tt.expectedLen, len(podMetrics))
			}

			// Verify each pod metric
			for _, pm := range podMetrics {
				if pm.pod == nil {
					t.Error("pod metric has nil pod")
				}
				if pm.podID == "" {
					t.Error("pod metric has empty podID")
				}
				if pm.doneCh == nil {
					t.Error("pod metric has nil doneCh")
				}

				// Verify podID matches expected format
				expectedID := pm.pod.Namespace + "/" + pm.pod.Name
				if pm.podID != expectedID {
					t.Errorf("expected podID %q, got %q", expectedID, pm.podID)
				}
			}

			// Verify all expected IDs are present
			if len(tt.expectedIDs) > 0 {
				foundIDs := make(map[string]bool)
				for _, pm := range podMetrics {
					foundIDs[pm.podID] = true
				}

				for _, expectedID := range tt.expectedIDs {
					if !foundIDs[expectedID] {
						t.Errorf("expected to find podID %q but it was not present", expectedID)
					}
				}
			}
		})
	}
}

// containsString is a helper to check if a string contains a substring
func containsString(s, substr string) bool {
	if len(s) == 0 || len(substr) == 0 {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
