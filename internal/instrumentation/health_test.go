package instrumentation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/newrelic/k8s-agents-operator/api/current"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ HealthCheck = (*fakeHealthCheck)(nil)

type fakeHealthCheck func(ctx context.Context, url string) (health Health, err error)

func (f fakeHealthCheck) GetHealth(ctx context.Context, url string) (health Health, err error) {
	return f(ctx, url)
}

var _ InstrumentationStatusUpdater = (*fakeUpdateInstrumentationStatus)(nil)

type fakeUpdateInstrumentationStatus func(ctx context.Context, instrumentation *current.Instrumentation) error

func (f fakeUpdateInstrumentationStatus) UpdateInstrumentationStatus(ctx context.Context, instrumentation *current.Instrumentation) error {
	return f(ctx, instrumentation)
}

func TestHealthMonitor(t *testing.T) {
	containerRestartPolicyAlways := corev1.ContainerRestartPolicyAlways
	tests := []struct {
		name                           string
		fnHealthCheck                  HealthCheck
		fnInstrumentationStatusUpdater InstrumentationStatusUpdater
		namespaces                     map[string]*corev1.Namespace
		pods                           map[string]*corev1.Pod
		instrumentations               map[string]*current.Instrumentation
		expectedInstrumentationStatus  current.InstrumentationStatus
	}{
		{
			name: "no health agent configured",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, nil
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {ObjectMeta: metav1.ObjectMeta{Name: "pod0", Namespace: "default"}},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic"},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching: 1,
			},
		},
		{
			name: "matching but not injected",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, nil
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {ObjectMeta: metav1.ObjectMeta{Name: "pod0", Namespace: "default"}},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic"},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching: 1,
			},
		},
		{
			name: "matching and injected but not ready and outdated",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, nil
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {ObjectMeta: metav1.ObjectMeta{Name: "pod0", Namespace: "default", Annotations: map[string]string{"newrelic.com/apm-health": "true"}}},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic"},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching: 1,
				PodsNotReady: 1,
				PodsOutdated: 1,
				PodsInjected: 1,
			},
		},
		{
			name: "matching and injected but not ready",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, nil
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {ObjectMeta: metav1.ObjectMeta{Name: "pod0", Namespace: "default", Annotations: map[string]string{
					"newrelic.com/apm-health":        "true",
					instrumentationVersionAnnotation: `{"newrelic/instrumentation0":"01234567-89ab-cdef-0123-456789abcdef/55"}`,
				}}},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching: 1,
				PodsNotReady: 1,
				PodsInjected: 1,
			},
		},
		{
			name: "matching, injected and ready, but missing health sidecar",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, fmt.Errorf("fake health check error, url: %q", url)
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod0", Namespace: "default", Annotations: map[string]string{
							"newrelic.com/apm-health":        "true",
							instrumentationVersionAnnotation: `{"newrelic/instrumentation0":"01234567-89ab-cdef-0123-456789abcdef/55"}`,
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []current.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed to identify health urls > health sidecar not found"}},
			},
		},
		{
			name: "matching, injected and ready, has the health sidecar, but no exposed ports",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, fmt.Errorf("fake health check error, url: %q", url)
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod0", Namespace: "default", Annotations: map[string]string{
							"newrelic.com/apm-health":        "true",
							instrumentationVersionAnnotation: `{"newrelic/instrumentation0":"01234567-89ab-cdef-0123-456789abcdef/55"}`,
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								RestartPolicy: &containerRestartPolicyAlways,
								Name:          "nri-health--something",
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []current.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed to identify health urls > health sidecar missing exposed ports"}},
			},
		},
		{
			name: "matching, injected and ready, has the health sidecar, but too many exposed ports",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, fmt.Errorf("fake health check error, url: %q", url)
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod0", Namespace: "default", Annotations: map[string]string{
							"newrelic.com/apm-health":        "true",
							instrumentationVersionAnnotation: `{"newrelic/instrumentation0":"01234567-89ab-cdef-0123-456789abcdef/55"}`,
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								RestartPolicy: &containerRestartPolicyAlways,
								Name:          "nri-health--something",
								Ports: []corev1.ContainerPort{
									{ContainerPort: 5678},
									{ContainerPort: 1234},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []current.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed to identify health urls > health sidecar has too many exposed ports"}},
			},
		},
		{
			name: "matching, injected and ready, has the health sidecar with only 1 exposed port",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, fmt.Errorf("fake health check error, url: %q", url)
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod0", Namespace: "default", Annotations: map[string]string{
							"newrelic.com/apm-health":        "true",
							instrumentationVersionAnnotation: `{"newrelic/instrumentation0":"01234567-89ab-cdef-0123-456789abcdef/55"}`,
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								RestartPolicy: &containerRestartPolicyAlways,
								Name:          "nri-health--something",
								Ports: []corev1.ContainerPort{
									{ContainerPort: 5678},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: "127.0.0.1",
					},
				},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []current.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed while retrieving health > fake health check error, url: \"http://127.0.0.1:5678/healthz\""}},
			},
		},
		{
			name: "matching, injected and ready, has the health sidecar with only 1 exposed port",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{EntityGUID: "1bad-f00d", Healthy: true}, nil
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				logger := log.FromContext(ctx)
				logger.Info("fake instrumentation status updater")
				return nil
			}),
			namespaces: map[string]*corev1.Namespace{
				"default":  {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				"newrelic": {ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			pods: map[string]*corev1.Pod{
				"default/pod0": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod0", Namespace: "default", Annotations: map[string]string{
							"newrelic.com/apm-health":        "true",
							instrumentationVersionAnnotation: `{"newrelic/instrumentation0":"01234567-89ab-cdef-0123-456789abcdef/55"}`,
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								RestartPolicy: &containerRestartPolicyAlways,
								Name:          "nri-health--something",
								Ports: []corev1.ContainerPort{
									{ContainerPort: 5678},
								},
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						PodIP: "127.0.0.1",
					},
				},
			},
			instrumentations: map[string]*current.Instrumentation{
				"newrelic/instrumentation0": {
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55},
					Spec:       current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health"}},
				},
			},
			expectedInstrumentationStatus: current.InstrumentationStatus{
				PodsMatching: 1,
				PodsInjected: 1,
				PodsHealthy:  1,
				EntityGUIDs:  []string{"1bad-f00d"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zapr.NewLogger(zaptest.NewLogger(t))
			ctx = logr.NewContext(ctx, logger)
			doneCh := make(chan struct{})
			var instrumentationStatus current.InstrumentationStatus
			waitForUpdateInstrumentationStatus := fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *current.Instrumentation) error {
				defer close(doneCh)
				err := test.fnInstrumentationStatusUpdater.UpdateInstrumentationStatus(ctx, instrumentation)
				if err != nil {
					return err
				}
				instrumentationStatus = instrumentation.Status
				return nil
			})
			hm := NewHealthMonitor(waitForUpdateInstrumentationStatus, test.fnHealthCheck, time.Millisecond*3, 50, 50, 2)
			toCtx, toCtxCancel := context.WithTimeout(ctx, time.Millisecond*5000)
			defer toCtxCancel()
			for _, namespace := range test.namespaces {
				hm.NamespaceSet(namespace)
			}
			for _, pod := range test.pods {
				hm.PodSet(pod)
			}
			for _, instrumentation := range test.instrumentations {
				hm.InstrumentationSet(instrumentation)
			}
			select {
			case <-doneCh:
				_ = hm.Shutdown(context.Background())
			case <-toCtx.Done():
				_ = hm.Stop(toCtx)
				t.Fatal("toCtx timed out")
			}
			if diff := cmp.Diff(test.expectedInstrumentationStatus, instrumentationStatus, cmpopts.IgnoreFields(current.InstrumentationStatus{}, "LastUpdated")); diff != "" {
				t.Errorf("unexpected status, got, want: %v", diff)
			}
		})
	}
}

func TestIsDiff(t *testing.T) {
	tests := []struct {
		name     string
		metric   instrumentationMetric
		expected error
	}{
		{
			name: "no diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{}},
			},
		},
		{
			name: "pods injected is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{PodsInjected: 6}},
				podsInjected:    5,
			},
			expected: errPodsInjectedIsDiff,
		},
		{
			name: "pods outdated is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{PodsOutdated: 5}},
				podsOutdated:    4,
			},
			expected: errPodsOutdatedIsDiff,
		},
		{
			name: "pods matching is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{PodsMatching: 4}},
				podsMatching:    3,
			},
			expected: errPodsMatchingIsDiff,
		},
		{
			name: "pods healthy is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{PodsHealthy: 3}},
				podsHealthy:     2,
			},
			expected: errPodsHealthyIsDiff,
		},
		{
			name: "pods unhealthy is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{PodsUnhealthy: 2}},
				podsUnhealthy:   1,
			},
			expected: errPodsUnhealthyIsDiff,
		},
		{
			name: "observed version is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "abc"}, Spec: current.InstrumentationSpec{}, Status: current.InstrumentationStatus{}},
			},
			expected: errObservedVersionIsDiff,
		},
		{
			name: "entity ids is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{EntityGUIDs: []string{"1bad-f00d"}}},
				entityGUIDs:     []string{"6ood-f00d"},
			},
			expected: errEntityGUIDIsDiff,
		},
		{
			name: "unhealthy pod errors is diff",
			metric: instrumentationMetric{
				instrumentation: &current.Instrumentation{Status: current.InstrumentationStatus{UnhealthyPodsErrors: []current.UnhealthyPodError{{Pod: "b"}}}},
				unhealthyPods:   []current.UnhealthyPodError{{Pod: "a"}},
			},
			expected: errUnhealthyPodErrorsIsDiff,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.metric.isDiff()
			if tc.expected != nil && actual != nil && !errors.Is(tc.expected, actual) {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
			if tc.expected == nil && actual != nil {
				t.Errorf("expected nil, got %v", actual)
			}
			if tc.expected != nil && actual == nil {
				t.Errorf("expected %v, got nil", tc.expected)
			}
		})
	}
}

func TestHasInstrumentationAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name:     "pod with no annotations",
			pod:      &corev1.Pod{},
			expected: false,
		},
		{
			name: "pod with empty annotations map",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "pod with instrumentation version annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"uid/1"}`,
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with other annotations but not instrumentation version",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"some-other-annotation": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with instrumentation version and other annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test":"uid/1"}`,
						"other-annotation":               "value",
					},
				},
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &HealthMonitor{}
			actual := m.hasInstrumentationAnnotation(test.pod)
			if actual != test.expected {
				t.Errorf("expected %v, got %v", test.expected, actual)
			}
		})
	}
}

func TestIsPodOutdated(t *testing.T) {
	tests := []struct {
		name            string
		pod             *corev1.Pod
		instrumentation *current.Instrumentation
		expected        bool
	}{
		{
			name:            "pod with no annotations",
			pod:             &corev1.Pod{},
			instrumentation: &current.Instrumentation{},
			expected:        true,
		},
		{
			name: "pod with empty annotations map",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			instrumentation: &current.Instrumentation{},
			expected:        true,
		},
		{
			name: "pod without instrumentation version annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			instrumentation: &current.Instrumentation{},
			expected:        true,
		},
		{
			name: "pod with invalid JSON in version annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `invalid json`,
					},
				},
			},
			instrumentation: &current.Instrumentation{},
			expected:        true,
		},
		{
			name: "pod missing specific instrumentation key",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/other-inst":"uid/1"}`,
					},
				},
			},
			instrumentation: &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 1,
				},
			},
			expected: true,
		},
		{
			name: "pod with matching UID and Generation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/5"}`,
					},
				},
			},
			instrumentation: &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 5,
				},
			},
			expected: false,
		},
		{
			name: "pod with outdated UID",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"old-uid/5"}`,
					},
				},
			},
			instrumentation: &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "new-uid",
					Generation: 5,
				},
			},
			expected: true,
		},
		{
			name: "pod with outdated Generation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/3"}`,
					},
				},
			},
			instrumentation: &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 5,
				},
			},
			expected: true,
		},
		{
			name: "pod with newer Generation (shouldn't happen but handle gracefully)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/test-inst":"test-uid/10"}`,
					},
				},
			},
			instrumentation: &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 5,
				},
			},
			expected: true,
		},
		{
			name: "pod with multiple instrumentations, one matching",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"default/inst1":"uid1/1","default/test-inst":"test-uid/3","default/inst2":"uid2/2"}`,
					},
				},
			},
			instrumentation: &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 3,
				},
			},
			expected: false,
		},
		{
			name: "pod with instrumentation from different namespace",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						instrumentationVersionAnnotation: `{"kube-system/test-inst":"test-uid/5"}`,
					},
				},
			},
			instrumentation: &current.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-inst",
					Namespace:  "default",
					UID:        "test-uid",
					Generation: 5,
				},
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &HealthMonitor{}
			actual := m.isPodOutdated(test.pod, test.instrumentation)
			if actual != test.expected {
				t.Errorf("expected %v, got %v", test.expected, actual)
			}
		})
	}
}
