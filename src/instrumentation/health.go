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
)

const (
	instrumentationVersionAnnotation = "newrelic.com/instrumentation-versions"
	healthSidecarContainerName       = "newrelic-apm-health"
	healthUrlFormat                  = "http://%s:%d/healthz"
	podMetricWorkers                 = 50
	instrumentationMetricWorkers     = 2
	instrumentationMetricWaiters     = 50
	maxBufferedEvents                = 100
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
	healthChecksDone
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

type HealthMonitor struct {
	instrumentationStatusUpdater InstrumentationStatusUpdater
	instrumentations             map[string]*v1alpha2.Instrumentation
	pods                         map[string]*corev1.Pod
	namespaces                   map[string]*corev1.Namespace
	events                       chan event
	doneCh                       chan struct{}
	doneOnce                     *sync.Once
	healthApi                    HealthCheck

	instrumentationMetricWorkers int
	podMetricWorkers             int
	instrumentationMetricWaiters int
	healthCheckTimeout           time.Duration

	podMetricsBatchCh                     chan []*podMetric
	instrumentationMetricsBatchCh         chan []*instrumentationMetric
	waitForInstrumentationsMetricsBatchCh chan []*instrumentationMetric

	tickInterval time.Duration

	shutdownCh   chan struct{}
	shutdownOnce *sync.Once

	stopCh   chan struct{}
	stopOnce *sync.Once

	pending int64
}

func NewHealthMonitor(
	ctx context.Context,
	instrumentationStatusUpdater InstrumentationStatusUpdater,
	healthCheck HealthCheck,
	tickInterval time.Duration,
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

		doneCh:   make(chan struct{}),
		doneOnce: &sync.Once{},

		events: make(chan event, maxBufferedEvents),

		healthCheckTimeout: healthCheckTimeout,
		tickInterval:       tickInterval,

		podMetricWorkers:                      podMetricWorkers,
		instrumentationMetricWorkers:          instrumentationMetricWorkers,
		instrumentationMetricWaiters:          instrumentationMetricWaiters,
		podMetricsBatchCh:                     make(chan []*podMetric),
		instrumentationMetricsBatchCh:         make(chan []*instrumentationMetric),
		waitForInstrumentationsMetricsBatchCh: make(chan []*instrumentationMetric),
	}
	m.start(ctx)
	return m
}

func (m *HealthMonitor) Shutdown(ctx context.Context) error {
	m.shutdownOnce.Do(func() {
		close(m.shutdownCh)
	})
	return m.waitUntilDone(ctx)
}

func (m *HealthMonitor) Stop(ctx context.Context) error {
	m.stopOnce.Do(func() {
		close(m.stopCh)
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
	m.sendEvent(event{pod: pod, action: podSet})
}

func (m *HealthMonitor) PodRemove(pod *corev1.Pod) {
	m.sendEvent(event{pod: pod, action: podRemove})
}

func (m *HealthMonitor) NamespaceSet(ns *corev1.Namespace) {
	m.sendEvent(event{ns: ns, action: nsSet})
}

func (m *HealthMonitor) NamespaceRemove(ns *corev1.Namespace) {
	m.sendEvent(event{ns: ns, action: nsRemove})
}

func (m *HealthMonitor) InstrumentationSet(instrumentation *v1alpha2.Instrumentation) {
	m.sendEvent(event{inst: instrumentation, action: instSet})
}

func (m *HealthMonitor) InstrumentationRemove(instrumentation *v1alpha2.Instrumentation) {
	m.sendEvent(event{inst: instrumentation, action: instRemove})
}

func (m *HealthMonitor) triggerHealthCheckDone() {
	m.sendEvent(event{action: healthChecksDone})
}

func (m *HealthMonitor) sendEvent(evt event) {
	select {
	case <-m.stopCh:
		return
	default:
	}
	atomic.AddInt64(&m.pending, 1)
	defer atomic.AddInt64(&m.pending, -1)
	select {
	case <-m.stopCh:
		return
	default:
	}
	select {
	case <-m.stopCh:
		return
	case m.events <- evt:
	}
}

func (m *HealthMonitor) onNamespaceSet(ev event) {
	m.namespaces[ev.ns.Name] = ev.ns
}

func (m *HealthMonitor) onNamespaceRemove(ev event) {
	if ns, ok := m.namespaces[ev.ns.Name]; ok {
		ns.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
	delete(m.namespaces, ev.ns.Name)
}

func (m *HealthMonitor) onPodSet(ev event) {
	id := types.NamespacedName{Namespace: ev.pod.Namespace, Name: ev.pod.Name}.String()
	m.pods[id] = ev.pod
}

func (m *HealthMonitor) onPodRemove(ev event) {
	id := types.NamespacedName{Namespace: ev.pod.Namespace, Name: ev.pod.Name}.String()
	if pod, ok := m.pods[id]; ok {
		pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
	delete(m.pods, id)
}

func (m *HealthMonitor) onInstrumentationSet(ev event) {
	id := types.NamespacedName{Namespace: ev.inst.Namespace, Name: ev.inst.Name}.String()
	m.instrumentations[id] = ev.inst
}

func (m *HealthMonitor) onInstrumentationRemove(ev event) {
	id := types.NamespacedName{Namespace: ev.inst.Namespace, Name: ev.inst.Name}.String()
	if inst, ok := m.instrumentations[id]; ok {
		inst.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
	delete(m.instrumentations, id)
}

func (m *HealthMonitor) startInstrumentationMetricsCalculator(ctx context.Context) {
	instrumentationMetricsPendingCh := make(chan *instrumentationMetric)
	instrumentationMetricsReadyCh := make(chan *instrumentationMetric)

	wg := sync.WaitGroup{}
	for i := 0; i < m.instrumentationMetricWaiters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.waitForPodMetricsForInstrumentationMetrics(ctx, instrumentationMetricsPendingCh, instrumentationMetricsReadyCh)
		}()
	}
	go func() {
		wg.Wait()
		close(instrumentationMetricsReadyCh)
	}()
	defer close(instrumentationMetricsPendingCh)
	for i := 0; i < m.instrumentationMetricWorkers; i++ {
		go m.instrumentationMetricsCalculationWorker(ctx, instrumentationMetricsReadyCh)
	}

	for {
		select {
		case <-m.stopCh:
			return
		case instrumentationMetricsBatch := <-m.instrumentationMetricsBatchCh:
			for _, instrumentationMetricItem := range instrumentationMetricsBatch {
				select {
				case <-m.stopCh:
					return
				case instrumentationMetricsPendingCh <- instrumentationMetricItem:
				}
			}
		}
	}
}

func (m *HealthMonitor) waitForPodMetricsForInstrumentationMetrics(ctx context.Context, instrumentationMetricsPendingCh chan *instrumentationMetric, instrumentationMetricsReadyCh chan *instrumentationMetric) {
	for instrumentationMetricItem := range instrumentationMetricsPendingCh {
		for _, podMetricItem := range instrumentationMetricItem.podMetrics {
			select {
			case <-m.stopCh:
				return
			case <-podMetricItem.doneCh:
			}
		}
		select {
		case <-m.stopCh:
			return
		case instrumentationMetricsReadyCh <- instrumentationMetricItem:
		}
	}
}

func (m *HealthMonitor) instrumentationMetricsCalculationWorker(ctx context.Context, instrumentationMetricsCh chan *instrumentationMetric) {
	for {
		select {
		case <-m.stopCh:
			return
		case instrumentationMetricItem := <-instrumentationMetricsCh:
			m.updateInstrumentation(ctx, instrumentationMetricItem)
		}
	}
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

func (m *HealthMonitor) updateInstrumentation(ctx context.Context, instrumentationMetric *instrumentationMetric) {
	defer close(instrumentationMetric.doneCh)
	logger := log.FromContext(ctx)
	for _, podMetricItem := range instrumentationMetric.podMetrics {
		instrumentationMetric.podsMatching++
		if !m.isPodInstrumented(podMetricItem.pod) {
			continue
		}
		if m.isPodOutdated(podMetricItem.pod, instrumentationMetric.instrumentation) {
			instrumentationMetric.podsOutdated++
		}
		instrumentationMetric.podsInjected++
		if !m.isPodReady(podMetricItem.pod) {
			instrumentationMetric.podsNotReady++
			continue
		}

		if podMetricItem.health.Healthy {
			instrumentationMetric.podsHealthy++
		} else {
			instrumentationMetric.podsUnhealthy++
			instrumentationMetric.unhealthyPods = append(instrumentationMetric.unhealthyPods, v1alpha2.UnhealthyPodError{
				Pod:       podMetricItem.podID,
				LastError: podMetricItem.health.LastError,
			})
		}
	}

	if instrumentationMetric.isDiff() {
		instrumentationMetric.syncStatus()
		instrumentationMetric.instrumentation.Status.LastUpdated = metav1.Now()

		select {
		case <-m.stopCh:
			return
		default:
		}

		if err := m.instrumentationStatusUpdater.UpdateInstrumentationStatus(ctx, instrumentationMetric.instrumentation); err != nil {
			logger.Error(err, "failed to update status for instrumentation")
		}
		logger.Info("wrote status for instrumentation", "id", instrumentationMetric.instrumentationID)
	} else {
		logger.Info("no changes to status for instrumentation", "id", instrumentationMetric.instrumentationID)
	}
}

func (m *HealthMonitor) startPodMetricsCalculator(ctx context.Context) {
	podMetricsCh := make(chan *podMetric)
	defer close(podMetricsCh)
	for i := 0; i < m.podMetricWorkers; i++ {
		go m.podMetricsCalculationWorker(ctx, podMetricsCh)
	}
	for {
		select {
		case <-m.stopCh:
			return
		case podMetrics := <-m.podMetricsBatchCh:
			for _, podMetricItem := range podMetrics {
				select {
				case <-m.stopCh:
					return
				case podMetricsCh <- podMetricItem:
				}
			}
		}
	}
}

func (m *HealthMonitor) podMetricsCalculationWorker(ctx context.Context, podMetricsCh chan *podMetric) {
	for {
		select {
		case <-m.stopCh:
			return
		case podMetricItem := <-podMetricsCh:
			m.check(ctx, podMetricItem)
		}
	}
}

func (m *HealthMonitor) startWaitForInstrumentationMetricsDoneWorker(ctx context.Context) {
	for {
		select {
		case <-m.stopCh:
			return
		case instrumentationMetrics := <-m.waitForInstrumentationsMetricsBatchCh:
			for _, instrumentationMetricItem := range instrumentationMetrics {
				select {
				case <-m.stopCh:
					return
				case <-instrumentationMetricItem.doneCh:
				}
			}
			m.triggerHealthCheckDone()
		}
	}
}

func (m *HealthMonitor) start(ctx context.Context) {
	wg := sync.WaitGroup{}
	wg.Add(4)

	go func() {
		defer wg.Done()
		defer close(m.podMetricsBatchCh)
		m.startPodMetricsCalculator(ctx)
	}()
	go func() {
		defer wg.Done()
		defer close(m.instrumentationMetricsBatchCh)
		m.startInstrumentationMetricsCalculator(ctx)
	}()
	go func() {
		defer wg.Done()
		defer close(m.waitForInstrumentationsMetricsBatchCh)
		m.startWaitForInstrumentationMetricsDoneWorker(ctx)
	}()
	go func() {
		defer wg.Done()
		m.startEventLoop(ctx)
	}()

	go func() {
		wg.Wait()
		close(m.events)
		close(m.doneCh)
	}()

	logger := log.FromContext(ctx)
	logger.Info("started")
}

func (m *HealthMonitor) startEventLoop(ctx context.Context) {
	logger := log.FromContext(ctx)
	ticker := time.NewTicker(m.tickInterval)
	defer ticker.Stop()
	ticksToSkip := 0

	healthCheckStart := time.Time{}
	healthCheckIsActive := false

	logger.Info("event loop started", "tick interval", m.tickInterval)
	for {
		select {
		case <-m.stopCh:
			return
		case _ = <-ticker.C:
			logger.Info("tick")
			if ticksToSkip > 0 {
				ticksToSkip--
				continue
			}
			if healthCheckIsActive {
				continue
			}
			logger.Info("health check start")
			podMetrics := m.getPodMetrics(ctx)
			if len(podMetrics) == 0 {
				logger.Info("nothing to check the health of.  No pods")
				continue
			}
			instrumentationMetrics := m.getInstrumentationMetrics(ctx, podMetrics)
			if len(instrumentationMetrics) == 0 {
				logger.Info("nothing to report the health to.  No instrumentations")
				continue
			}

			logger.Info("starting the scheduling of health checks")
			healthCheckStart = time.Now()
			healthCheckIsActive = true

			m.podMetricsBatchCh <- podMetrics
			m.instrumentationMetricsBatchCh <- instrumentationMetrics
			m.waitForInstrumentationsMetricsBatchCh <- instrumentationMetrics

			logger.Info("finished scheduling health checks")

		case ev := <-m.events:
			logger.Info("event", "action", ev.action)
			switch ev.action {
			case healthChecksDone:
				logger.Info("health check done")
				healthCheckIsActive = false
				totalTime := time.Since(healthCheckStart)

				// skip a tick (or more) if the time it takes exceeds our interval. round off the extra, otherwise we always skip at least 1
				ticksToSkip = int(totalTime / m.tickInterval)
				if ticksToSkip > 0 {
					logger.Info(fmt.Sprintf("Skipping %d ticks at a interval of %s", ticksToSkip, m.tickInterval.String()))
				}

				drainingTicker := true
				for drainingTicker {
					select {
					case <-ticker.C:
					default:
						drainingTicker = false
					}
				}
			case nsSet:
				m.onNamespaceSet(ev)
			case nsRemove:
				m.onNamespaceRemove(ev)
			case podSet:
				m.onPodSet(ev)
			case podRemove:
				m.onPodRemove(ev)
			case instSet:
				m.onInstrumentationSet(ev)
			case instRemove:
				m.onInstrumentationRemove(ev)
			}
		}
	}
}

func (m *HealthMonitor) check(ctx context.Context, podMetricItem *podMetric) {
	logger := log.FromContext(ctx)
	defer func() {
		logger.Info("closing pod metric", "pod", podMetricItem.podID)
		close(podMetricItem.doneCh)
	}()
	if !m.isPodInstrumented(podMetricItem.pod) {
		return
	}
	if !m.isPodReady(podMetricItem.pod) {
		return
	}
	logger.Info("checking health for pod", "pod", podMetricItem.podID)

	pod := podMetricItem.pod
	podHealthUrl, err := m.getHealthUrlFromPod(*pod)
	if err != nil {
		podMetricItem.health = Health{
			Healthy:   false,
			LastError: fmt.Sprintf("failed to identify health url > %s", err.Error()),
		}
		return
	}

	healthCtx, healthCtxCancel := context.WithTimeout(ctx, m.healthCheckTimeout)
	defer healthCtxCancel()
	health, err := m.healthApi.GetHealth(healthCtx, podHealthUrl)
	if err != nil {
		podMetricItem.health = Health{
			Healthy:   false,
			LastError: fmt.Sprintf("failed while retrieving health > %s", err.Error()),
		}
		return
	}
	logger.Info("collected health for pod", "pod", podMetricItem.podID, "health", health)
	podMetricItem.health = health

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

func (m *HealthMonitor) getHealthUrlFromPod(pod corev1.Pod) (string, error) {
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
