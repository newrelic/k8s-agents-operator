package instrumentation

import (
	"context"
	"testing"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/apm"
	"github.com/newrelic/k8s-agents-operator/internal/instrumentation/util/worker"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// verifyMixedHealthConfigs is a helper to verify health check configs for multiple instrumentations
func verifyMixedHealthConfigs(t *testing.T, instMetrics []*instrumentationMetric) {
	t.Helper()
	for _, im := range instMetrics {
		inst := im.instrumentation
		switch inst.Name {
		case "inst-no-health":
			if im.healthCheckEnabled {
				t.Errorf("instrumentation 'inst-no-health' should have healthCheckEnabled=false")
			}
		case "inst-with-health":
			if !im.healthCheckEnabled {
				t.Errorf("instrumentation 'inst-with-health' should have healthCheckEnabled=true")
			}
		}
	}
}

// TestGetInstrumentationMetrics_WithoutHealthAgent tests that instrumentations
// without HealthAgent configured are still tracked in metrics (Option 2 implementation)
func TestGetInstrumentationMetrics_WithoutHealthAgent(t *testing.T) {
	tests := []struct {
		name                      string
		instrumentations          map[string]*current.Instrumentation
		namespaces                map[string]*corev1.Namespace
		pods                      map[string]*corev1.Pod
		expectedCount             int
		expectedHealthCheckEnable bool
	}{
		{
			name: "instrumentation without health agent is tracked",
			instrumentations: map[string]*current.Instrumentation{
				"default/inst-no-health": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst-no-health",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "java",
							Image:    "java-agent:latest",
						},
						// No HealthAgent configured
						HealthAgent: current.HealthAgent{},
					},
				},
			},
			namespaces: map[string]*corev1.Namespace{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedCount:             1,
			expectedHealthCheckEnable: false,
		},
		{
			name: "instrumentation with health agent has healthCheckEnabled=true",
			instrumentations: map[string]*current.Instrumentation{
				"default/inst-with-health": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst-with-health",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "java",
							Image:    "java-agent:latest",
						},
						HealthAgent: current.HealthAgent{
							Image: "health-agent:latest",
						},
					},
				},
			},
			namespaces: map[string]*corev1.Namespace{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedCount:             1,
			expectedHealthCheckEnable: true,
		},
		{
			name: "multiple instrumentations with mixed health agent configs",
			instrumentations: map[string]*current.Instrumentation{
				"default/inst-no-health": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst-no-health",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "app1"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "java",
							Image:    "java-agent:latest",
						},
						// No HealthAgent
						HealthAgent: current.HealthAgent{},
					},
				},
				"default/inst-with-health": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst-with-health",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "app2"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "python",
							Image:    "python-agent:latest",
						},
						HealthAgent: current.HealthAgent{
							Image: "health-agent:latest",
						},
					},
				},
			},
			namespaces: map[string]*corev1.Namespace{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels:    map[string]string{"app": "app1"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				"default/pod2": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "default",
						Labels:    map[string]string{"app": "app2"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedCount: 2,
			// Will check each individually
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			m := &HealthMonitor{
				instrumentations: tt.instrumentations,
				namespaces:       tt.namespaces,
			}

			// Create pod metrics
			podMetrics := make([]*podMetric, 0, len(tt.pods))
			for _, pod := range tt.pods {
				podMetrics = append(podMetrics, &podMetric{
					pod: pod,
				})
			}

			// Get instrumentation metrics
			instMetrics := m.getInstrumentationMetrics(ctx, podMetrics)

			// Verify count
			if len(instMetrics) != tt.expectedCount {
				t.Errorf("expected %d instrumentation metrics, got %d", tt.expectedCount, len(instMetrics))
			}

			// For single instrumentation tests, verify healthCheckEnabled
			if tt.expectedCount == 1 {
				if instMetrics[0].healthCheckEnabled != tt.expectedHealthCheckEnable {
					t.Errorf("expected healthCheckEnabled=%v, got %v", tt.expectedHealthCheckEnable, instMetrics[0].healthCheckEnabled)
				}
			}

			// For multiple instrumentations test, verify each one
			if tt.name == "multiple instrumentations with mixed health agent configs" {
				verifyMixedHealthConfigs(t, instMetrics)
			}
		})
	}
}

// TestInstrumentationMetricQueueEvent_HealthCheckEnabled tests that the health check
// flag properly controls whether health metrics are collected (Option 2 implementation)
func TestInstrumentationMetricQueueEvent_HealthCheckEnabled(t *testing.T) {
	tests := []struct {
		name                  string
		pod                   *corev1.Pod
		healthCheckEnabled    bool
		podHasAnnotation      bool
		podReady              bool
		healths               []Health
		expectedPodsMatching  int64
		expectedPodsInjected  int64
		expectedPodsHealthy   int64
		expectedPodsUnhealthy int64
		expectedPodsNotReady  int64
		expectedPodsOutdated  int64
	}{
		{
			name: "pod tracked without health agent",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`,
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled:    false,
			podHasAnnotation:      true,
			podReady:              true,
			healths:               []Health{}, // No health checks
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0, // Not tracked when healthCheckEnabled=false
			expectedPodsUnhealthy: 0,
			expectedPodsNotReady:  0,
			expectedPodsOutdated:  0,
		},
		{
			name: "pod tracked with health agent - healthy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`,
						apm.HealthInstrumentedAnnotation: "true",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled: true,
			podHasAnnotation:   true,
			podReady:           true,
			healths: []Health{
				{
					Healthy:    true,
					EntityGUID: "entity-guid-1",
				},
			},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   1,
			expectedPodsUnhealthy: 0,
			expectedPodsNotReady:  0,
			expectedPodsOutdated:  0,
		},
		{
			name: "pod tracked with health agent - unhealthy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`,
						apm.HealthInstrumentedAnnotation: "true",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled: true,
			podHasAnnotation:   true,
			podReady:           true,
			healths: []Health{
				{
					Healthy:   false,
					LastError: "health check failed",
				},
			},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 1,
			expectedPodsNotReady:  0,
			expectedPodsOutdated:  0,
		},
		{
			name: "pod not ready - no health metrics",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`,
						apm.HealthInstrumentedAnnotation: "true",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			healthCheckEnabled:    true,
			podHasAnnotation:      true,
			podReady:              false,
			healths:               []Health{},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 0,
			expectedPodsNotReady:  1,
			expectedPodsOutdated:  0,
		},
		{
			name: "pod without instrumentation annotation - not injected",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pod1",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled:    false,
			podHasAnnotation:      false,
			podReady:              true,
			healths:               []Health{},
			expectedPodsMatching:  1,
			expectedPodsInjected:  0, // Not injected
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 0,
			expectedPodsNotReady:  0,
			expectedPodsOutdated:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create instrumentation
			inst := &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 1,
				},
			}

			// Create pod metric
			pm := &podMetric{
				pod:     tt.pod,
				healths: tt.healths,
				doneCh:  make(chan struct{}),
			}
			close(pm.doneCh) // Mark as done so wait() doesn't block

			// Create instrumentation metric
			event := &instrumentationMetric{
				instrumentationID:  "default/test-inst",
				instrumentation:    inst,
				podMetrics:         []*podMetric{pm},
				healthCheckEnabled: tt.healthCheckEnabled,
			}

			// Create monitor with required queue
			m := &HealthMonitor{
				instrumentationMetricPersistQueue: worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
					// No-op worker for test
				}),
			}

			// Process the event
			m.instrumentationMetricQueueEvent(ctx, event)

			// Verify results
			if event.podsMatching != tt.expectedPodsMatching {
				t.Errorf("expected podsMatching=%d, got %d", tt.expectedPodsMatching, event.podsMatching)
			}
			if event.podsInjected != tt.expectedPodsInjected {
				t.Errorf("expected podsInjected=%d, got %d", tt.expectedPodsInjected, event.podsInjected)
			}
			if event.podsHealthy != tt.expectedPodsHealthy {
				t.Errorf("expected podsHealthy=%d, got %d", tt.expectedPodsHealthy, event.podsHealthy)
			}
			if event.podsUnhealthy != tt.expectedPodsUnhealthy {
				t.Errorf("expected podsUnhealthy=%d, got %d", tt.expectedPodsUnhealthy, event.podsUnhealthy)
			}
			if event.podsNotReady != tt.expectedPodsNotReady {
				t.Errorf("expected podsNotReady=%d, got %d", tt.expectedPodsNotReady, event.podsNotReady)
			}
			if event.podsOutdated != tt.expectedPodsOutdated {
				t.Errorf("expected podsOutdated=%d, got %d", tt.expectedPodsOutdated, event.podsOutdated)
			}
		})
	}
}

// TestGetInstrumentationMetrics_EdgeCases tests edge cases in instrumentation metrics collection
func TestGetInstrumentationMetrics_EdgeCases(t *testing.T) {
	tests := []struct {
		name                   string
		instrumentations       map[string]*current.Instrumentation
		namespaces             map[string]*corev1.Namespace
		pods                   map[string]*corev1.Pod
		expectedCount          int
		expectedMatchingCounts map[string]int // instrumentation name -> expected matching pods
	}{
		{
			name: "multiple instrumentations matching same pod",
			instrumentations: map[string]*current.Instrumentation{
				"default/inst1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst1",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "java",
							Image:    "java-agent:latest",
						},
					},
				},
				"default/inst2": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst2",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"}, // Same selector
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "python",
							Image:    "python-agent:latest",
						},
					},
				},
			},
			namespaces: map[string]*corev1.Namespace{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedCount: 2, // Both instrumentations should have metrics
			expectedMatchingCounts: map[string]int{
				"inst1": 1,
				"inst2": 1,
			},
		},
		{
			name: "pod in namespace not in monitor's namespace map",
			instrumentations: map[string]*current.Instrumentation{
				"default/inst1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst1",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "java",
							Image:    "java-agent:latest",
						},
					},
				},
			},
			namespaces: map[string]*corev1.Namespace{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
				// "other-ns" is not in the map
			},
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				"other-ns/pod2": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "other-ns", // Namespace not in monitor
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedCount: 1,
			expectedMatchingCounts: map[string]int{
				"inst1": 1, // Only pod1 from default namespace should match
			},
		},
		{
			name: "namespace selector filters pods",
			instrumentations: map[string]*current.Instrumentation{
				"default/inst1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst1",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Agent: current.Agent{
							Language: "java",
							Image:    "java-agent:latest",
						},
					},
				},
			},
			namespaces: map[string]*corev1.Namespace{
				"prod-ns": {
					ObjectMeta: metav1.ObjectMeta{
						Name:   "prod-ns",
						Labels: map[string]string{"env": "prod"},
					},
				},
				"dev-ns": {
					ObjectMeta: metav1.ObjectMeta{
						Name:   "dev-ns",
						Labels: map[string]string{"env": "dev"},
					},
				},
			},
			pods: map[string]*corev1.Pod{
				"prod-ns/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "prod-ns",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				"dev-ns/pod2": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "dev-ns",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedCount: 1,
			expectedMatchingCounts: map[string]int{
				"inst1": 1, // Only pod1 from prod-ns should match
			},
		},
		{
			name: "no pods match instrumentation selectors",
			instrumentations: map[string]*current.Instrumentation{
				"default/inst1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inst1",
						Namespace: "default",
					},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nonexistent"},
						},
						NamespaceLabelSelector: metav1.LabelSelector{},
						Agent: current.Agent{
							Language: "java",
							Image:    "java-agent:latest",
						},
					},
				},
			},
			namespaces: map[string]*corev1.Namespace{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			pods: map[string]*corev1.Pod{
				"default/pod1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedCount: 1,
			expectedMatchingCounts: map[string]int{
				"inst1": 0, // No pods should match
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			m := &HealthMonitor{
				instrumentations: tt.instrumentations,
				namespaces:       tt.namespaces,
			}

			// Create pod metrics
			podMetrics := make([]*podMetric, 0, len(tt.pods))
			for _, pod := range tt.pods {
				podMetrics = append(podMetrics, &podMetric{
					pod: pod,
				})
			}

			// Get instrumentation metrics
			instMetrics := m.getInstrumentationMetrics(ctx, podMetrics)

			// Verify count
			if len(instMetrics) != tt.expectedCount {
				t.Errorf("expected %d instrumentation metrics, got %d", tt.expectedCount, len(instMetrics))
			}

			// Verify matching counts
			for _, im := range instMetrics {
				instName := im.instrumentation.Name
				expectedCount, ok := tt.expectedMatchingCounts[instName]
				if !ok {
					continue
				}
				actualCount := len(im.podMetrics)
				if actualCount != expectedCount {
					t.Errorf("instrumentation %q: expected %d matching pods, got %d", instName, expectedCount, actualCount)
				}
			}
		})
	}
}

// TestInstrumentationMetricQueueEvent_EdgeCases tests edge cases in metric queue event processing
func TestInstrumentationMetricQueueEvent_EdgeCases(t *testing.T) {
	tests := []struct {
		name                  string
		pod                   *corev1.Pod
		healthCheckEnabled    bool
		healths               []Health
		expectedPodsMatching  int64
		expectedPodsInjected  int64
		expectedPodsHealthy   int64
		expectedPodsUnhealthy int64
		expectedPodsOutdated  int64
	}{
		{
			name: "pod with outdated instrumentation version",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"old-uid/1"}`, // Different UID
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled:    false,
			healths:               []Health{},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 0,
			expectedPodsOutdated:  1, // Should be marked as outdated
		},
		{
			name: "pod with outdated generation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/0"}`, // Old generation
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled:    false,
			healths:               []Health{},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 0,
			expectedPodsOutdated:  1, // Should be marked as outdated
		},
		{
			name: "pod with current version",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`, // Current
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled:    false,
			healths:               []Health{},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 0,
			expectedPodsOutdated:  0, // Should NOT be marked as outdated
		},
		{
			name: "pod with multiple health URLs - all healthy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`,
						apm.HealthInstrumentedAnnotation: "true",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled: true,
			healths: []Health{
				{Healthy: true, EntityGUID: "entity-1"},
				{Healthy: true, EntityGUID: "entity-2"},
			},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   1, // All healthy
			expectedPodsUnhealthy: 0,
			expectedPodsOutdated:  0,
		},
		{
			name: "pod with multiple health URLs - one unhealthy",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`,
						apm.HealthInstrumentedAnnotation: "true",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled: true,
			healths: []Health{
				{Healthy: true, EntityGUID: "entity-1"},
				{Healthy: false, LastError: "check failed"},
			},
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 1, // One unhealthy
			expectedPodsOutdated:  0,
		},
		{
			name: "pod with empty health results",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/1"}`,
						apm.HealthInstrumentedAnnotation: "true",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			healthCheckEnabled:    true,
			healths:               []Health{}, // Empty - no health data yet
			expectedPodsMatching:  1,
			expectedPodsInjected:  1,
			expectedPodsHealthy:   0,
			expectedPodsUnhealthy: 0, // Not counted as unhealthy, just no data
			expectedPodsOutdated:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create instrumentation
			inst := &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 1,
				},
			}

			// Create pod metric
			pm := &podMetric{
				pod:     tt.pod,
				healths: tt.healths,
				doneCh:  make(chan struct{}),
			}
			close(pm.doneCh) // Mark as done so wait() doesn't block

			// Create instrumentation metric
			event := &instrumentationMetric{
				instrumentationID:  "default/test-inst",
				instrumentation:    inst,
				podMetrics:         []*podMetric{pm},
				healthCheckEnabled: tt.healthCheckEnabled,
				doneCh:             make(chan struct{}),
			}

			// Create monitor with required queue
			m := &HealthMonitor{
				instrumentationMetricPersistQueue: worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
					// No-op worker for test
				}),
			}

			// Process the event
			m.instrumentationMetricQueueEvent(ctx, event)

			// Verify results
			if event.podsMatching != tt.expectedPodsMatching {
				t.Errorf("expected podsMatching=%d, got %d", tt.expectedPodsMatching, event.podsMatching)
			}
			if event.podsInjected != tt.expectedPodsInjected {
				t.Errorf("expected podsInjected=%d, got %d", tt.expectedPodsInjected, event.podsInjected)
			}
			if event.podsHealthy != tt.expectedPodsHealthy {
				t.Errorf("expected podsHealthy=%d, got %d", tt.expectedPodsHealthy, event.podsHealthy)
			}
			if event.podsUnhealthy != tt.expectedPodsUnhealthy {
				t.Errorf("expected podsUnhealthy=%d, got %d", tt.expectedPodsUnhealthy, event.podsUnhealthy)
			}
			if event.podsOutdated != tt.expectedPodsOutdated {
				t.Errorf("expected podsOutdated=%d, got %d", tt.expectedPodsOutdated, event.podsOutdated)
			}
		})
	}
}
