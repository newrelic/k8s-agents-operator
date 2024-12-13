package instrumentation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
	"github.com/newrelic/k8s-agents-operator/src/apm"
	"github.com/newrelic/k8s-agents-operator/src/instrumentation/util/ticker"
	"github.com/newrelic/k8s-agents-operator/src/instrumentation/util/worker"
)

const (
	instrumentationVersionAnnotation = "newrelic.com/instrumentation-versions"
	healthSidecarContainerName       = "newrelic-apm-health-sidecar"
	healthUrlFormat                  = "http://%s:%d/healthz"
)

var healthCheckTimeout = time.Second * 15

type eventAction int

const (
	podSet eventAction = iota
	podRemove
	instSet
	instRemove
	nsSet
	nsRemove
	triggerHealthCheck
)

type event struct {
	action eventAction
	pod    *corev1.Pod
	ns     *corev1.Namespace
	inst   *v1alpha2.Instrumentation
}

type instrumentationMetric struct {
	instrumentationID string
	instrumentation   *v1alpha2.Instrumentation
	podMetrics        []*podMetric
	doneCh            chan struct{}
	podsMatching      int64
	podsInjected      int64
	podsNotReady      int64
	podsOutdated      int64
	podsHealthy       int64
	podsUnhealthy     int64
	unhealthyPods     []v1alpha2.UnhealthyPodError
}

func (im *instrumentationMetric) resolve() {
	close(im.doneCh)
}

func (im *instrumentationMetric) wait(ctx context.Context) error {
	select {
	case <-im.doneCh:
		return nil
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-im.doneCh:
		return nil
	}
}

func (im *instrumentationMetric) isDiff() bool {
	if im.instrumentation.Status.PodsInjected != im.podsInjected {
		return true
	}
	if im.instrumentation.Status.PodsOutdated != im.podsOutdated {
		return true
	}
	if im.instrumentation.Status.PodsMatching != im.podsMatching {
		return true
	}
	if im.instrumentation.Status.PodsHealthy != im.podsHealthy {
		return true
	}
	if im.instrumentation.Status.PodsUnhealthy != im.podsUnhealthy {
		return true
	}
	sort.Slice(im.unhealthyPods, func(i, j int) bool {
		if im.unhealthyPods[i].Pod < im.unhealthyPods[j].Pod {
			return true
		}
		return false
	})
	return !reflect.DeepEqual(im.unhealthyPods, im.instrumentation.Status.UnhealthyPodsErrors)
}

func (im *instrumentationMetric) syncStatus() {
	im.instrumentation.Status.PodsOutdated = im.podsOutdated
	im.instrumentation.Status.PodsInjected = im.podsInjected
	im.instrumentation.Status.PodsMatching = im.podsMatching
	im.instrumentation.Status.PodsOutdated = im.podsOutdated
	im.instrumentation.Status.PodsNotReady = im.podsNotReady
	im.instrumentation.Status.PodsHealthy = im.podsHealthy
	im.instrumentation.Status.PodsUnhealthy = im.podsUnhealthy
	im.instrumentation.Status.UnhealthyPodsErrors = im.unhealthyPods
}

type podMetric struct {
	pod              *corev1.Pod
	podID            string
	needsHealthCheck bool
	health           Health
	doneCh           chan struct{}
}

func (pm *podMetric) resolve(health Health) {
	pm.health = health
	close(pm.doneCh)
}

func (pm *podMetric) wait(ctx context.Context) error {
	select {
	case <-pm.doneCh:
		return nil
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-pm.doneCh:
		return nil
	}
}

type healthCheckData struct {
	podMetrics             []*podMetric
	instrumentationMetrics []*instrumentationMetric
}

type HealthMonitor struct {
	healthCheckActive int64
	_padding1         [3]int64

	checksToSkip int64
	_padding2    [3]int64

	doneCh   chan struct{}
	doneOnce *sync.Once

	shutdownCh   chan struct{}
	shutdownOnce *sync.Once

	stopCh   chan struct{}
	stopOnce *sync.Once

	healthApi                         HealthCheck
	instrumentationStatusUpdater      InstrumentationStatusUpdater
	resourceQueue                     worker.Worker
	healthCheckQueue                  worker.Worker
	podMetricsQueue                   worker.Worker
	podMetricQueue                    worker.Worker
	instrumentationMetricsQueue       worker.Worker
	instrumentationMetricQueue        worker.Worker
	instrumentationMetricPersistQueue worker.Worker
	ticker                            *ticker.Ticker

	instrumentations map[string]*v1alpha2.Instrumentation
	pods             map[string]*corev1.Pod
	namespaces       map[string]*corev1.Namespace

	healthCheckTimeout time.Duration
	tickInterval       time.Duration
}

func NewHealthMonitor(
	instrumentationStatusUpdater InstrumentationStatusUpdater,
	healthCheck HealthCheck,
	tickInterval time.Duration,
	podMetricWorkers int,
	instrumentationsMetricWorkers int,
	instrumentationsMetricPersistWorkers int,
) *HealthMonitor {
	m := &HealthMonitor{
		healthApi:                    healthCheck,
		instrumentationStatusUpdater: instrumentationStatusUpdater,

		instrumentations: make(map[string]*v1alpha2.Instrumentation),
		pods:             make(map[string]*corev1.Pod),
		namespaces:       make(map[string]*corev1.Namespace),

		shutdownCh:   make(chan struct{}),
		shutdownOnce: &sync.Once{},
		stopCh:       make(chan struct{}),
		stopOnce:     &sync.Once{},
		doneCh:       make(chan struct{}),
		doneOnce:     &sync.Once{},

		healthCheckTimeout: healthCheckTimeout,
		tickInterval:       tickInterval,
	}
	m.ticker = ticker.NewTicker(tickInterval, func(ctx context.Context, _ time.Time) {
		m.resourceQueue.Add(ctx, event{action: triggerHealthCheck})
	})
	m.resourceQueue = worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
		m.resourceQueueEvent(ctx, data.(event))
	})
	m.healthCheckQueue = worker.NewManyWorkers(2, 0, func(ctx context.Context, data any) {
		m.healthCheckQueueEvent(ctx, data.(healthCheckData))
	})
	m.podMetricsQueue = worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
		m.podMetricsQueueEvent(ctx, data.([]*podMetric))
	})
	m.podMetricQueue = worker.NewManyWorkers(podMetricWorkers, 0, func(ctx context.Context, data any) {
		m.podMetricQueueEvent(ctx, data.(*podMetric))
	})
	m.instrumentationMetricsQueue = worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
		m.instrumentationMetricsQueueEvent(ctx, data.([]*instrumentationMetric))
	})
	m.instrumentationMetricQueue = worker.NewManyWorkers(instrumentationsMetricWorkers, 0, func(ctx context.Context, data any) {
		m.instrumentationMetricQueueEvent(ctx, data.(*instrumentationMetric))
	})
	m.instrumentationMetricPersistQueue = worker.NewManyWorkers(instrumentationsMetricPersistWorkers, 0, func(ctx context.Context, data any) {
		m.instrumentationMetricPersistQueueEvent(ctx, data.(*instrumentationMetric))
	})
	return m
}

func (m *HealthMonitor) resourceQueueEvent(ctx context.Context, ev event) {
	logger := log.FromContext(ctx)
	logger.Info("event", "action", ev.action)
	switch ev.action {
	case nsSet:
		m.namespaces[ev.ns.Name] = ev.ns
	case nsRemove:
		delete(m.namespaces, ev.ns.Name)
	case podSet:
		m.pods[ev.pod.Namespace+"/"+ev.pod.Name] = ev.pod
	case podRemove:
		delete(m.pods, ev.pod.Namespace+"/"+ev.pod.Name)
	case instSet:
		m.instrumentations[ev.inst.Namespace+"/"+ev.inst.Name] = ev.inst
	case instRemove:
		delete(m.instrumentations, ev.inst.Namespace+"/"+ev.inst.Name)
	case triggerHealthCheck:
		if atomic.LoadInt64(&m.healthCheckActive) == 1 {
			return
		}

		logger.Info("health check start")
		podMetrics := m.getPodMetrics(ctx)
		if len(podMetrics) == 0 {
			logger.Info("nothing to check the health of.  No pods")
			return
		}
		instrumentationMetrics := m.getInstrumentationMetrics(ctx, podMetrics)
		if len(instrumentationMetrics) == 0 {
			logger.Info("nothing to report the health to.  No instrumentations")
			return
		}
		m.healthCheckQueue.Add(ctx, healthCheckData{podMetrics: podMetrics, instrumentationMetrics: instrumentationMetrics})
	}
}

func (m *HealthMonitor) healthCheckQueueEvent(ctx context.Context, event healthCheckData) {
	// consume any extra ticks that might be waiting
	if atomic.SwapInt64(&m.healthCheckActive, 1) == 1 {
		return
	}
	defer atomic.StoreInt64(&m.healthCheckActive, 0)

	if m.checksToSkip > 0 {
		m.checksToSkip--
		return
	}

	logger := log.FromContext(ctx)
	healthCheckStartTime := time.Now()

	m.podMetricsQueue.Add(ctx, event.podMetrics)
	m.instrumentationMetricsQueue.Add(ctx, event.instrumentationMetrics)
	for _, eventInstrumentationMetric := range event.instrumentationMetrics {
		eventInstrumentationMetric.wait(ctx)
	}
	for _, eventPodMetric := range event.podMetrics {
		eventPodMetric.wait(ctx)
	}

	totalTime := time.Since(healthCheckStartTime)

	// skip a tick (or more) if the time it takes exceeds our interval. round off the extra, otherwise we always skip at least 1
	m.checksToSkip = int64(totalTime / m.tickInterval)
	if m.checksToSkip > 0 {
		logger.Info("Skipping health checks", "skip_count", m.checksToSkip, "interval", m.tickInterval.String())
	}
}

func (m *HealthMonitor) podMetricsQueueEvent(ctx context.Context, event []*podMetric) {
	for _, eventPodMetric := range event {
		m.podMetricQueue.Add(ctx, eventPodMetric)
	}
}

func (m *HealthMonitor) podMetricQueueEvent(ctx context.Context, event *podMetric) {
	health := m.check(ctx, event)
	event.resolve(health)
}

func (m *HealthMonitor) instrumentationMetricsQueueEvent(ctx context.Context, event []*instrumentationMetric) {
	for _, eventInstrumentationMetric := range event {
		m.instrumentationMetricQueue.Add(ctx, eventInstrumentationMetric)
	}
}

func (m *HealthMonitor) instrumentationMetricQueueEvent(ctx context.Context, event *instrumentationMetric) {
	for _, eventPodMetrics := range event.podMetrics {
		eventPodMetrics.wait(ctx)
		event.podsMatching++
		if !m.isPodInstrumented(eventPodMetrics.pod) {
			continue
		}
		if m.isPodOutdated(eventPodMetrics.pod, event.instrumentation) {
			event.podsOutdated++
		}
		event.podsInjected++
		if !m.isPodReady(eventPodMetrics.pod) {
			event.podsNotReady++
			continue
		}

		if eventPodMetrics.health.Healthy {
			event.podsHealthy++
		} else {
			event.podsUnhealthy++
			event.unhealthyPods = append(event.unhealthyPods, v1alpha2.UnhealthyPodError{
				Pod:       eventPodMetrics.podID,
				LastError: eventPodMetrics.health.LastError,
			})
		}
	}
	m.instrumentationMetricPersistQueue.Add(ctx, event)
}

func (m *HealthMonitor) instrumentationMetricPersistQueueEvent(ctx context.Context, event *instrumentationMetric) {
	defer event.resolve()
	logger := log.FromContext(ctx)
	if event.isDiff() {
		event.syncStatus()
		event.instrumentation.Status.LastUpdated = metav1.Now()

		select {
		case <-m.stopCh:
			return
		default:
		}

		if err := m.instrumentationStatusUpdater.UpdateInstrumentationStatus(ctx, event.instrumentation); err != nil {
			logger.Error(err, "failed to update status for instrumentation")
		}
		logger.Info("wrote status for instrumentation", "id", event.instrumentationID)
	} else {
		logger.Info("no changes to status for instrumentation", "id", event.instrumentationID)
	}
}

func (m *HealthMonitor) getPodMetrics(ctx context.Context) []*podMetric {
	podMetrics := make([]*podMetric, len(m.pods))
	i := 0
	for _, pod := range m.pods {
		podMetrics[i] = &podMetric{
			pod:    pod,
			podID:  types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}.String(),
			doneCh: make(chan struct{}),
		}
		i++
	}
	return podMetrics[0:i]
}

func (m *HealthMonitor) getInstrumentationMetrics(ctx context.Context, podMetrics []*podMetric) []*instrumentationMetric {
	logger := log.FromContext(ctx)
	var instrumentationMetrics = make([]*instrumentationMetric, len(m.instrumentations))
	i := 0
	matchedPodMetricIDs := map[string]struct{}{}
	for _, instrumentation := range m.instrumentations {
		podSelector, err := metav1.LabelSelectorAsSelector(&instrumentation.Spec.PodLabelSelector)
		if err != nil {
			logger.Error(err, "failed to parse instrumentation pod selector",
				"instrumentation", types.NamespacedName{Namespace: instrumentation.Namespace, Name: instrumentation.Name}.String(),
			)
			continue
		}
		namespaceSelector, err := metav1.LabelSelectorAsSelector(&instrumentation.Spec.NamespaceLabelSelector)
		if err != nil {
			logger.Error(err, "failed to parse instrumentation namespace selector",
				"instrumentation", types.NamespacedName{Namespace: instrumentation.Namespace, Name: instrumentation.Name}.String(),
			)
			continue
		}

		var instPodMetrics []*podMetric
		for _, podMetricItem := range podMetrics {
			ns, ok := m.namespaces[podMetricItem.pod.Namespace]
			if !ok {
				continue
			}
			if !namespaceSelector.Matches(labels.Set(ns.Labels)) {
				continue
			}
			if !podSelector.Matches(labels.Set(podMetricItem.pod.Labels)) {
				continue
			}
			instPodMetrics = append(instPodMetrics, podMetricItem)
			matchedPodMetricIDs[podMetricItem.podID] = struct{}{}
		}

		instrumentationMetrics[i] = &instrumentationMetric{
			instrumentationID: types.NamespacedName{Namespace: instrumentation.Namespace, Name: instrumentation.Name}.String(),
			instrumentation:   instrumentation,
			podMetrics:        instPodMetrics,
			doneCh:            make(chan struct{}),
		}
		i++
	}

	return instrumentationMetrics[0:i]
}

func (m *HealthMonitor) getHealthUrlFromPod(pod *corev1.Pod) (string, error) {
	var sidecars []corev1.Container
	for _, container := range pod.Spec.InitContainers {
		if container.RestartPolicy == nil {
			continue
		}
		if *container.RestartPolicy != corev1.ContainerRestartPolicyAlways {
			continue
		}
		if container.Name != healthSidecarContainerName {
			continue
		}
		sidecars = append(sidecars, container)
	}
	if len(sidecars) == 0 {
		return "", fmt.Errorf("health sidecar not found")
	}
	sidecar := sidecars[0]
	if len(sidecar.Ports) == 0 {
		return "", fmt.Errorf("health sidecar missing exposed ports")
	}
	if len(sidecar.Ports) > 1 {
		return "", fmt.Errorf("health sidecar has too many exposed ports")
	}
	return fmt.Sprintf(healthUrlFormat, pod.Status.PodIP, sidecar.Ports[0].ContainerPort), nil
}

func (m *HealthMonitor) isPodReady(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}

func (m *HealthMonitor) isPodOutdated(pod *corev1.Pod, inst *v1alpha2.Instrumentation) bool {
	v, ok := pod.Annotations[instrumentationVersionAnnotation]
	if !ok {
		return true
	}
	instVersions := map[string]string{}
	err := json.Unmarshal([]byte(v), &instVersions)
	if err != nil {
		return true
	}
	podInstVersion, ok := instVersions[types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}.String()]
	if !ok {
		return true
	}
	if instVersion := fmt.Sprintf("%s/%d", inst.UID, inst.Generation); podInstVersion != instVersion {
		return true
	}
	return false
}

func (m *HealthMonitor) isPodInstrumented(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	value, ok := pod.Annotations[apm.HealthInstrumentedAnnotation]
	if !ok {
		return false
	}
	bv, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return bv
}

func (m *HealthMonitor) check(ctx context.Context, podMetricItem *podMetric) Health {
	logger := log.FromContext(ctx)
	if !m.isPodInstrumented(podMetricItem.pod) {
		return Health{}
	}
	if !m.isPodReady(podMetricItem.pod) {
		return Health{}
	}
	logger.Info("checking health for pod", "pod", podMetricItem.podID)

	podHealthUrl, err := m.getHealthUrlFromPod(podMetricItem.pod)
	if err != nil {
		return Health{
			Healthy:   false,
			LastError: fmt.Sprintf("failed to identify health url > %s", err.Error()),
		}
	}

	healthCtx, healthCtxCancel := context.WithTimeout(ctx, m.healthCheckTimeout)
	defer healthCtxCancel()
	health, err := m.healthApi.GetHealth(healthCtx, podHealthUrl)
	if err != nil {
		return Health{
			Healthy:   false,
			LastError: fmt.Sprintf("failed while retrieving health > %s", err.Error()),
		}
	}
	logger.Info("collected health for pod", "pod", podMetricItem.podID, "health", health)
	return health
}

func (m *HealthMonitor) Shutdown(ctx context.Context) error {
	m.shutdownOnce.Do(func() {
		go func() {
			defer m.doneOnce.Do(func() { close(m.doneCh) })
			ctxStop := context.Background()
			m.ticker.Stop(ctxStop)
			m.resourceQueue.Stop(ctxStop)
			m.healthCheckQueue.Shutdown(ctxStop)
			m.podMetricsQueue.Stop(ctxStop)
			m.podMetricQueue.Stop(ctxStop)
			m.instrumentationMetricsQueue.Stop(ctxStop)
			m.instrumentationMetricQueue.Stop(ctxStop)
			m.instrumentationMetricPersistQueue.Stop(ctxStop)
		}()
	})
	return m.waitUntilDone(ctx)
}

func (m *HealthMonitor) Stop(ctx context.Context) error {
	m.stopOnce.Do(func() {
		go func() {
			defer m.doneOnce.Do(func() { close(m.doneCh) })
			ctxStop := context.Background()
			m.ticker.Stop(ctxStop)
			m.resourceQueue.Stop(ctxStop)
			m.healthCheckQueue.Stop(ctxStop)
			m.podMetricsQueue.Stop(ctxStop)
			m.podMetricQueue.Stop(ctxStop)
			m.instrumentationMetricsQueue.Stop(ctxStop)
			m.instrumentationMetricQueue.Stop(ctxStop)
			m.instrumentationMetricPersistQueue.Stop(ctxStop)
		}()
	})
	return m.waitUntilDone(ctx)
}

func (m *HealthMonitor) waitUntilDone(ctx context.Context) error {
	// intentionally left here for predictable thread scheduling
	select {
	case <-m.doneCh:
	default:
	}

	select {
	case <-m.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (m *HealthMonitor) PodSet(pod *corev1.Pod) {
	m.resourceQueue.Add(context.Background(), event{pod: pod, action: podSet})
}

func (m *HealthMonitor) PodRemove(pod *corev1.Pod) {
	m.resourceQueue.Add(context.Background(), event{pod: pod, action: podRemove})
}

func (m *HealthMonitor) NamespaceSet(ns *corev1.Namespace) {
	m.resourceQueue.Add(context.Background(), event{ns: ns, action: nsSet})
}

func (m *HealthMonitor) NamespaceRemove(ns *corev1.Namespace) {
	m.resourceQueue.Add(context.Background(), event{ns: ns, action: nsRemove})
}

func (m *HealthMonitor) InstrumentationSet(instrumentation *v1alpha2.Instrumentation) {
	m.resourceQueue.Add(context.Background(), event{inst: instrumentation, action: instSet})
}

func (m *HealthMonitor) InstrumentationRemove(instrumentation *v1alpha2.Instrumentation) {
	m.resourceQueue.Add(context.Background(), event{inst: instrumentation, action: instRemove})
}
