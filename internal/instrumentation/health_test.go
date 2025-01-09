package instrumentation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/newrelic/k8s-agents-operator/api/v1alpha2"
)

var _ HealthCheck = (*fakeHealthCheck)(nil)

type fakeHealthCheck func(ctx context.Context, url string) (health Health, err error)

func (f fakeHealthCheck) GetHealth(ctx context.Context, url string) (health Health, err error) {
	return f(ctx, url)
}

var _ InstrumentationStatusUpdater = (*fakeUpdateInstrumentationStatus)(nil)

type fakeUpdateInstrumentationStatus func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error

func (f fakeUpdateInstrumentationStatus) UpdateInstrumentationStatus(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
		instrumentations               map[string]*v1alpha2.Instrumentation
		expectedInstrumentationStatus  v1alpha2.InstrumentationStatus
	}{
		{
			name: "matching but not injected",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, nil
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
			instrumentations: map[string]*v1alpha2.Instrumentation{
				"newrelic/instrumentation0": {ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic"}},
			},
			expectedInstrumentationStatus: v1alpha2.InstrumentationStatus{
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
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
			instrumentations: map[string]*v1alpha2.Instrumentation{
				"newrelic/instrumentation0": {ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic"}},
			},
			expectedInstrumentationStatus: v1alpha2.InstrumentationStatus{
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
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
			instrumentations: map[string]*v1alpha2.Instrumentation{
				"newrelic/instrumentation0": {ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55}},
			},
			expectedInstrumentationStatus: v1alpha2.InstrumentationStatus{
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
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
			instrumentations: map[string]*v1alpha2.Instrumentation{
				"newrelic/instrumentation0": {ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55}},
			},
			expectedInstrumentationStatus: v1alpha2.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []v1alpha2.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed to identify health url > health sidecar not found"}},
			},
		},
		{
			name: "matching, injected and ready, has the health sidecar, but no exposed ports",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, fmt.Errorf("fake health check error, url: %q", url)
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
								Name:          healthSidecarContainerName,
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			instrumentations: map[string]*v1alpha2.Instrumentation{
				"newrelic/instrumentation0": {ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55}},
			},
			expectedInstrumentationStatus: v1alpha2.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []v1alpha2.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed to identify health url > health sidecar missing exposed ports"}},
			},
		},
		{
			name: "matching, injected and ready, has the health sidecar, but too many exposed ports",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, fmt.Errorf("fake health check error, url: %q", url)
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
								Name:          healthSidecarContainerName,
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
			instrumentations: map[string]*v1alpha2.Instrumentation{
				"newrelic/instrumentation0": {ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55}},
			},
			expectedInstrumentationStatus: v1alpha2.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []v1alpha2.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed to identify health url > health sidecar has too many exposed ports"}},
			},
		},
		{
			name: "matching, injected and ready, has the health sidecar with only 1 exposed port",
			fnHealthCheck: fakeHealthCheck(func(ctx context.Context, url string) (health Health, err error) {
				logger := log.FromContext(ctx)
				logger.Info("fake health check")
				return Health{}, fmt.Errorf("fake health check error, url: %q", url)
			}),
			fnInstrumentationStatusUpdater: fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
								Name:          healthSidecarContainerName,
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
			instrumentations: map[string]*v1alpha2.Instrumentation{
				"newrelic/instrumentation0": {ObjectMeta: metav1.ObjectMeta{Name: "instrumentation0", Namespace: "newrelic", UID: "01234567-89ab-cdef-0123-456789abcdef", Generation: 55}},
			},
			expectedInstrumentationStatus: v1alpha2.InstrumentationStatus{
				PodsMatching:        1,
				PodsInjected:        1,
				PodsUnhealthy:       1,
				UnhealthyPodsErrors: []v1alpha2.UnhealthyPodError{{Pod: "default/pod0", LastError: "failed while retrieving health > fake health check error, url: \"http://127.0.0.1:5678/healthz\""}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			logger := zapr.NewLogger(zaptest.NewLogger(t))
			ctx = logr.NewContext(ctx, logger)
			doneCh := make(chan struct{})
			var instrumentationStatus v1alpha2.InstrumentationStatus
			waitForUpdateInstrumentationStatus := fakeUpdateInstrumentationStatus(func(ctx context.Context, instrumentation *v1alpha2.Instrumentation) error {
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
			if diff := cmp.Diff(test.expectedInstrumentationStatus, instrumentationStatus, cmpopts.IgnoreFields(v1alpha2.InstrumentationStatus{}, "LastUpdated")); diff != "" {
				t.Errorf("unexpected status, got, want: %v", diff)
			}
		})
	}
}
