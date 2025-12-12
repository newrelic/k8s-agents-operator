package instrumentation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/apm"
	"github.com/newrelic/k8s-agents-operator/internal/instrumentation/util/ticker"
	"github.com/newrelic/k8s-agents-operator/internal/instrumentation/util/worker"
)

const (
	instrumentationVersionAnnotation = "newrelic.com/instrumentation-versions"
	healthUrlFormat                  = "http://%s:%d/healthz"
)

var healthCheckTimeout = time.Second * 15

type eventAction int

// all possible actions in the event loop
const (
	podSet eventAction = iota
	podRemove
	instSet
	instRemove
	nsSet
	nsRemove
	triggerHealthCheck
)

// String to print the event name in the enum
func (ea eventAction) String() string {
	switch ea {
	case podSet:
		return "podSet"
	case podRemove:
		return "podRemove"
	case instSet:
		return "instSet"
	case instRemove:
		return "instRemove"
	case nsSet:
		return "nsSet"
	case nsRemove:
		return "nsRemove"
	case triggerHealthCheck:
		return "triggerHealthCheck"
	default:
		return "unknown"
	}
}

// event contains the event action (pod/namespace/instrumentation change or a health check) along with associated data if needed
type event struct {
	action eventAction
	pod    *corev1.Pod
	ns     *corev1.Namespace
	inst   *current.Instrumentation
}

// instrumentationMetric contains a copy(pointer to the copy) of the instrumentation, a copy(shared pointer copy) of
// each matching pod metric (pod + health) along with the aggregated summary of the health of all the pod metric health
type instrumentationMetric struct {
	instrumentationID string
	instrumentation   *current.Instrumentation
	podMetrics        []*podMetric
	doneCh            chan struct{}
	podsMatching      int64
	podsInjected      int64
	podsNotReady      int64
	podsOutdated      int64
	podsHealthy       int64
	podsUnhealthy     int64
	unhealthyPods     []current.UnhealthyPodError
	entityGUIDs       []string
}

// resolve marks the instrumentation metric done.  anything waiting via `wait` will continue
func (im *instrumentationMetric) resolve() {
	close(im.doneCh)
}

// wait will continue once `resolve` is called or the context expires, whichever happens first
func (im *instrumentationMetric) wait(ctx context.Context) error {
	// keep this block here.  for deterministic thread scheduling
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

var (
	errPodsInjectedIsDiff       = errors.New("podsInjected is diff")
	errPodsOutdatedIsDiff       = errors.New("podsOutdated is diff")
	errPodsMatchingIsDiff       = errors.New("podsMatching is diff")
	errPodsHealthyIsDiff        = errors.New("podsHealthy is diff")
	errPodsUnhealthyIsDiff      = errors.New("podsUnhealthy is diff")
	errObservedVersionIsDiff    = errors.New("observedVersion is diff")
	errEntityGUIDIsDiff         = errors.New("entityGUIDs is diff")
	errUnhealthyPodErrorsIsDiff = errors.New("unhealthyPodErrors is diff")
)

// isDiff to check for differences.  used to know if we need to write any data
func (im *instrumentationMetric) isDiff() error {
	if im.instrumentation.Status.PodsInjected != im.podsInjected {
		return errPodsInjectedIsDiff
	}
	if im.instrumentation.Status.PodsOutdated != im.podsOutdated {
		return errPodsOutdatedIsDiff
	}
	if im.instrumentation.Status.PodsMatching != im.podsMatching {
		return errPodsMatchingIsDiff
	}
	if im.instrumentation.Status.PodsHealthy != im.podsHealthy {
		return errPodsHealthyIsDiff
	}
	if im.instrumentation.Status.PodsUnhealthy != im.podsUnhealthy {
		return errPodsUnhealthyIsDiff
	}
	if im.instrumentation.Status.ObservedVersion != im.instrumentation.ResourceVersion {
		return errObservedVersionIsDiff
	}
	sort.Slice(im.unhealthyPods, func(i, j int) bool { return im.unhealthyPods[i].Pod < im.unhealthyPods[j].Pod })
	im.entityGUIDs = uniqueSlices(im.entityGUIDs)
	sort.Slice(im.entityGUIDs, func(i, j int) bool { return strings.Compare(im.entityGUIDs[i], im.entityGUIDs[j]) < 0 })
	if !reflect.DeepEqual(im.entityGUIDs, im.instrumentation.Status.EntityGUIDs) {
		return errEntityGUIDIsDiff
	}
	if !reflect.DeepEqual(im.unhealthyPods, im.instrumentation.Status.UnhealthyPodsErrors) {
		return errUnhealthyPodErrorsIsDiff
	}
	return nil
}

func uniqueSlices(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	u := map[string]struct{}{}
	for _, item := range in {
		if _, ok := u[item]; !ok {
			u[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

// syncStatus to copy the data from the instrumentation metric to the instrumentation
func (im *instrumentationMetric) syncStatus() {
	im.instrumentation.Status.PodsOutdated = im.podsOutdated
	im.instrumentation.Status.PodsInjected = im.podsInjected
	im.instrumentation.Status.PodsMatching = im.podsMatching
	im.instrumentation.Status.PodsOutdated = im.podsOutdated
	im.instrumentation.Status.PodsNotReady = im.podsNotReady
	im.instrumentation.Status.PodsHealthy = im.podsHealthy
	im.instrumentation.Status.PodsUnhealthy = im.podsUnhealthy
	im.instrumentation.Status.UnhealthyPodsErrors = im.unhealthyPods
	im.instrumentation.Status.ObservedVersion = im.instrumentation.ResourceVersion
	im.instrumentation.Status.EntityGUIDs = im.entityGUIDs
}

// podMetric contains the pod, it's id (used for logging), health (empty by default), and doneCh - which is closed once health has been retrieved
type podMetric struct {
	pod     *corev1.Pod
	podID   string
	healths []Health
	doneCh  chan struct{}
}

// resolve sets the health data and closes doneCh, which signals other processes (go routines) to continue
func (pm *podMetric) resolve(healths []Health) {
	pm.healths = healths
	close(pm.doneCh)
}

// wait will wait for doneCh to close or the context to expire, which ever occurs first.
func (pm *podMetric) wait(ctx context.Context) error {
	// check for doneCh for determinism while testing
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

// healthCheckData contains all the needed data for checking health in the cluster at a given moment in time
type healthCheckData struct {
	podMetrics             []*podMetric
	instrumentationMetrics []*instrumentationMetric
}

// HealthMonitor .
type HealthMonitor struct {
	healthCheckActive int64
	// cpu cache align healthCheckActive so that it can atomically change without affecting cache coherency
	_padding1 [3]int64

	checksToSkip int64
	// cpu cache align checksToSkip so that it can atomically change without affecting cache coherency
	_padding2 [3]int64

	// channel used to wait for all workers to finish when shutting down or closing
	doneCh chan struct{}
	// only close the doneCh channel once
	doneOnce     *sync.Once
	shutdownOnce *sync.Once
	stopOnce     *sync.Once

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

	instrumentations map[string]*current.Instrumentation
	pods             map[string]*corev1.Pod
	namespaces       map[string]*corev1.Namespace

	healthCheckTimeout time.Duration
	tickInterval       time.Duration
}

// NewHealthMonitor returns a new instance of a health monitor which check the health of pods via the health sidecar
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

		instrumentations: make(map[string]*current.Instrumentation),
		pods:             make(map[string]*corev1.Pod),
		namespaces:       make(map[string]*corev1.Namespace),

		shutdownOnce: &sync.Once{},
		stopOnce:     &sync.Once{},
		doneCh:       make(chan struct{}),
		doneOnce:     &sync.Once{},

		healthCheckTimeout: healthCheckTimeout,
		tickInterval:       tickInterval,
	}
	m.ticker = ticker.NewTicker(tickInterval, func(ctx context.Context, _ time.Time) {
		// schedule the health check of the cluster
		_ = m.resourceQueue.Add(ctx, event{action: triggerHealthCheck})
	})
	m.resourceQueue = worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
		// convert interface/any to correct object type
		m.resourceQueueEvent(ctx, data.(event))
	})
	m.healthCheckQueue = worker.NewManyWorkers(2, 0, func(ctx context.Context, data any) {
		// convert interface/any to correct object type
		m.healthCheckQueueEvent(ctx, data.(healthCheckData))
	})
	m.podMetricsQueue = worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
		// convert interface/any to correct object type
		m.podMetricsQueueEvent(ctx, data.([]*podMetric))
	})
	m.podMetricQueue = worker.NewManyWorkers(podMetricWorkers, 0, func(ctx context.Context, data any) {
		// convert interface/any to correct object type
		m.podMetricQueueEvent(ctx, data.(*podMetric))
	})
	m.instrumentationMetricsQueue = worker.NewManyWorkers(1, 0, func(ctx context.Context, data any) {
		// convert interface/any to correct object type
		m.instrumentationMetricsQueueEvent(ctx, data.([]*instrumentationMetric))
	})
	m.instrumentationMetricQueue = worker.NewManyWorkers(instrumentationsMetricWorkers, 0, func(ctx context.Context, data any) {
		// convert interface/any to correct object type
		m.instrumentationMetricQueueEvent(ctx, data.(*instrumentationMetric))
	})
	m.instrumentationMetricPersistQueue = worker.NewManyWorkers(instrumentationsMetricPersistWorkers, 0, func(ctx context.Context, data any) {
		// convert interface/any to correct object type
		m.instrumentationMetricPersistQueueEvent(ctx, data.(*instrumentationMetric))
	})
	return m
}

func (m *HealthMonitor) resourceQueueEvent(ctx context.Context, ev event) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("event", "action", ev.action)
	switch ev.action {
	case nsSet:
		logger.V(1).Info("event", "action", ev.action.String(), "entity", "namespace/"+ev.ns.Name)
		m.namespaces[ev.ns.Name] = ev.ns
	case nsRemove:
		logger.V(1).Info("event", "action", ev.action.String(), "entity", "namespace/"+ev.ns.Name)
		delete(m.namespaces, ev.ns.Name)
	case podSet:
		logger.V(1).Info("event", "action", ev.action.String(), "entity", "namespace/"+ev.pod.Namespace+"/pod/"+ev.pod.Name)
		m.pods[ev.pod.Namespace+"/"+ev.pod.Name] = ev.pod
	case podRemove:
		logger.V(1).Info("event", "action", ev.action.String(), "entity", "namespace/"+ev.pod.Namespace+"/pod/"+ev.pod.Name)
		delete(m.pods, ev.pod.Namespace+"/"+ev.pod.Name)
	case instSet:
		logger.V(1).Info("event", "action", ev.action.String(), "entity", "namespace/"+ev.inst.Namespace+"/instrumentation/"+ev.inst.Name)
		m.instrumentations[ev.inst.Namespace+"/"+ev.inst.Name] = ev.inst
	case instRemove:
		logger.V(1).Info("event", "action", ev.action.String(), "entity", "namespace/"+ev.inst.Namespace+"/instrumentation/"+ev.inst.Name)
		delete(m.instrumentations, ev.inst.Namespace+"/"+ev.inst.Name)
	case triggerHealthCheck:
		// skip health check if it's already active
		if atomic.LoadInt64(&m.healthCheckActive) == 1 {
			return
		}

		podMetrics := m.getPodMetrics(ctx)
		if len(podMetrics) == 0 {
			logger.V(1).Info("triggered a health check, but there's nothing to check the health of.  No pods")
			return
		}
		instrumentationMetrics := m.getInstrumentationMetrics(ctx, podMetrics)
		if len(instrumentationMetrics) == 0 {
			logger.V(1).Info("triggered a health check, but there's nothing to report the health to.  No instrumentations with a configured health agent")
			return
		}
		logger.V(1).Info("trigger health check")
		// use the required data at this point in time to do health checks
		_ = m.healthCheckQueue.Add(ctx, healthCheckData{podMetrics: podMetrics, instrumentationMetrics: instrumentationMetrics})
	}
}

func (m *HealthMonitor) healthCheckQueueEvent(ctx context.Context, event healthCheckData) {
	// swap the current active state with 1, if the returned state is 1 (active), we should return since it's already running
	if atomic.SwapInt64(&m.healthCheckActive, 1) == 1 {
		return
	}
	// set the active state to 0 (false) when this returns
	defer atomic.StoreInt64(&m.healthCheckActive, 0)

	// depending on how long this takes to run, if we exceed the interval duration (15sec), we will skip every health check after until we are at less than 50% utilization
	if m.checksToSkip > 0 {
		m.checksToSkip--
		return
	}

	logger := log.FromContext(ctx)
	healthCheckStartTime := time.Now()

	// send off pod metrics to another queue to be processed async
	_ = m.podMetricsQueue.Add(ctx, event.podMetrics)
	// send off instrumentation metrics to another queue to be processed async
	_ = m.instrumentationMetricsQueue.Add(ctx, event.instrumentationMetrics)
	// wait for all instrumentation metrics to be summarized and aggregated and finally persisted
	for _, eventInstrumentationMetric := range event.instrumentationMetrics {
		_ = eventInstrumentationMetric.wait(ctx)
	}
	// wait for any pod metrics to finish being collected.  even if nothing needs to be collected this will still complete
	for _, eventPodMetric := range event.podMetrics {
		_ = eventPodMetric.wait(ctx)
	}

	totalTime := time.Since(healthCheckStartTime)

	logger.Info("health check time", "duration", totalTime.String())

	// skip a tick (or more) if the time it takes exceeds our interval. round off the extra, otherwise we always skip at least 1
	m.checksToSkip = int64(totalTime / m.tickInterval)
	if m.checksToSkip > 0 {
		logger.Info("skipping health checks", "skip_count", m.checksToSkip, "interval", m.tickInterval.String())
	}
}

func (m *HealthMonitor) podMetricsQueueEvent(ctx context.Context, event []*podMetric) {
	// process each pod metric individually
	for _, eventPodMetric := range event {
		_ = m.podMetricQueue.Add(ctx, eventPodMetric)
	}
}

func (m *HealthMonitor) podMetricQueueEvent(ctx context.Context, event *podMetric) {
	// check the pod health, save the data in the pod metrics
	health := m.check(ctx, event)
	event.resolve(health)
}

func (m *HealthMonitor) instrumentationMetricsQueueEvent(ctx context.Context, event []*instrumentationMetric) {
	// calculate the instrumentation metrics individually
	for _, eventInstrumentationMetric := range event {
		_ = m.instrumentationMetricQueue.Add(ctx, eventInstrumentationMetric)
	}
}

func (m *HealthMonitor) instrumentationMetricQueueEvent(ctx context.Context, event *instrumentationMetric) {
	for _, eventPodMetrics := range event.podMetrics {
		// wait for pod metrics (health) to be collected
		_ = eventPodMetrics.wait(ctx)
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

		var entityGUIDs []string
		healthy := true
		var unhealthyPods []current.UnhealthyPodError
		for _, health := range eventPodMetrics.healths {
			if health.EntityGUID != "" {
				entityGUIDs = append(entityGUIDs, health.EntityGUID)
			}
			healthy = healthy && health.Healthy
			if !health.Healthy {
				unhealthyPods = append(unhealthyPods, current.UnhealthyPodError{
					Pod:       eventPodMetrics.podID,
					LastError: health.LastError,
				})
			}
		}
		event.entityGUIDs = entityGUIDs
		event.unhealthyPods = unhealthyPods
		if healthy {
			event.podsHealthy++
		} else {
			event.podsUnhealthy++
		}
	}
	// send our instrumentation metrics off to be persisted
	_ = m.instrumentationMetricPersistQueue.Add(ctx, event)
}

func (m *HealthMonitor) instrumentationMetricPersistQueueEvent(ctx context.Context, event *instrumentationMetric) {
	// mark instrumentation metrics done once the status has been persisted (if required)
	defer event.resolve()
	logger := log.FromContext(ctx).WithValues(
		"id", event.instrumentationID,
		"pods", map[string]int64{
			"matching":  event.podsMatching,
			"outdated":  event.podsOutdated,
			"unhealthy": event.podsUnhealthy,
			"not_ready": event.podsNotReady,
			"healthy":   event.podsHealthy,
			"injected":  event.podsInjected,
		},
	)
	if err := event.isDiff(); err != nil {
		event.syncStatus()
		event.instrumentation.Status.LastUpdated = metav1.Now()
		if err := m.instrumentationStatusUpdater.UpdateInstrumentationStatus(ctx, event.instrumentation); err != nil {
			logger.Error(err, "failed to update status for instrumentation")
		}
		logger.Info("wrote status for instrumentation")
	} else {
		logger.Info("no changes to status for instrumentation")
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

// getInstrumentationMetrics is used to find all pods that match the instrumentation selectors.  each instrumentation metric will have the associated pod metrics
func (m *HealthMonitor) getInstrumentationMetrics(ctx context.Context, podMetrics []*podMetric) []*instrumentationMetric {
	logger := log.FromContext(ctx)
	var instrumentationMetrics = make([]*instrumentationMetric, len(m.instrumentations))
	i := 0
	for _, instrumentation := range m.instrumentations {
		// skip instrumentation without the health agent configuration, because it's extra work without any benefit
		if instrumentation.Spec.HealthAgent.IsEmpty() {
			continue
		}

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

// getHealthUrlsFromPod is used to get the pod ip and port of the container for the health sidecar
func (m *HealthMonitor) getHealthUrlsFromPod(pod *corev1.Pod) ([]string, error) {
	var sidecars = make([]corev1.Container, 0, 1)
	for _, container := range pod.Spec.InitContainers {
		if container.RestartPolicy == nil {
			continue
		}
		if *container.RestartPolicy != corev1.ContainerRestartPolicyAlways {
			continue
		}
		if len(container.Name) < 12 || container.Name[:12] != "nri-health--" {
			continue
		}
		sidecars = append(sidecars, container)
	}
	if len(sidecars) == 0 {
		return nil, fmt.Errorf("health sidecar not found")
	}
	urls := make([]string, 0, len(sidecars))
	for _, sidecar := range sidecars {
		if len(sidecar.Ports) == 0 {
			return nil, fmt.Errorf("health sidecar missing exposed ports")
		}
		if len(sidecar.Ports) > 1 {
			return nil, fmt.Errorf("health sidecar has too many exposed ports")
		}
		urls = append(urls, fmt.Sprintf(healthUrlFormat, pod.Status.PodIP, sidecar.Ports[0].ContainerPort))
	}
	return urls, nil
}

// isPodReady is ued to calculate if a pod is ready
func (m *HealthMonitor) isPodReady(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}

// isPodOutdated is used to compare the instrumentation generation against the pod annotation of the instrumentation applied
func (m *HealthMonitor) isPodOutdated(pod *corev1.Pod, inst *current.Instrumentation) bool {
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

// isPodInstrumented check if a pod has been instrumented with the health sidecar
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

// check is used to check the health of the container, the health sidecar
func (m *HealthMonitor) check(ctx context.Context, podMetricItem *podMetric) []Health {
	logger := log.FromContext(ctx)
	if !m.isPodInstrumented(podMetricItem.pod) {
		return nil
	}
	if !m.isPodReady(podMetricItem.pod) {
		return nil
	}
	logger.V(2).Info("checking health for pod", "pod", podMetricItem.podID)

	podHealthUrls, err := m.getHealthUrlsFromPod(podMetricItem.pod)
	if err != nil {
		return []Health{{
			Healthy:   false,
			LastError: fmt.Sprintf("failed to identify health urls > %s", err.Error()),
		}}
	}

	healthCtx, healthCtxCancel := context.WithDeadline(ctx, time.Now().Add(m.healthCheckTimeout))
	defer healthCtxCancel()
	healths := make([]Health, 0, len(podHealthUrls))
	for _, podHealthUrl := range podHealthUrls {
		health, err := m.healthApi.GetHealth(healthCtx, podHealthUrl)
		if err != nil {
			return []Health{{
				Healthy:   false,
				LastError: fmt.Sprintf("failed while retrieving health > %s", err.Error()),
			}}
		}
		healths = append(healths, health)
	}
	logger.V(2).Info("collected health for pod", "pod", podMetricItem.podID, "healths", healths)
	return healths
}

// Shutdown is used to shutdown all new work.  anything already processing will continue.  this could be called multiple
// times.  Calling this will block until either it's finished or the context expires
func (m *HealthMonitor) Shutdown(ctx context.Context) error {
	m.shutdownOnce.Do(func() {
		go func() {
			defer m.doneOnce.Do(func() { close(m.doneCh) })
			ctxStop := context.Background()
			_ = m.ticker.Stop(ctxStop)
			_ = m.resourceQueue.Stop(ctxStop)
			_ = m.healthCheckQueue.Shutdown(ctxStop)
			_ = m.podMetricsQueue.Stop(ctxStop)
			_ = m.podMetricQueue.Stop(ctxStop)
			_ = m.instrumentationMetricsQueue.Stop(ctxStop)
			_ = m.instrumentationMetricQueue.Stop(ctxStop)
			_ = m.instrumentationMetricPersistQueue.Stop(ctxStop)
		}()
	})
	return m.waitUntilDone(ctx)
}

// Stop is used to stop all work.  this could be called multiple times.  Calling this will block until either it's
// finished or the context expires
func (m *HealthMonitor) Stop(ctx context.Context) error {
	m.stopOnce.Do(func() {
		go func() {
			defer m.doneOnce.Do(func() { close(m.doneCh) })
			ctxStop := context.Background()
			_ = m.ticker.Stop(ctxStop)
			_ = m.resourceQueue.Stop(ctxStop)
			_ = m.healthCheckQueue.Stop(ctxStop)
			_ = m.podMetricsQueue.Stop(ctxStop)
			_ = m.podMetricQueue.Stop(ctxStop)
			_ = m.instrumentationMetricsQueue.Stop(ctxStop)
			_ = m.instrumentationMetricQueue.Stop(ctxStop)
			_ = m.instrumentationMetricPersistQueue.Stop(ctxStop)
		}()
	})
	return m.waitUntilDone(ctx)
}

// waitUntilDone waits for the stop or shutdown to finish, or the context expires
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

// PodSet to set the pod
func (m *HealthMonitor) PodSet(pod *corev1.Pod) {
	_ = m.resourceQueue.Add(context.Background(), event{pod: pod, action: podSet})
}

// PodRemove to remove the pod
func (m *HealthMonitor) PodRemove(pod *corev1.Pod) {
	_ = m.resourceQueue.Add(context.Background(), event{pod: pod, action: podRemove})
}

// NamespaceSet to set the namespace
func (m *HealthMonitor) NamespaceSet(ns *corev1.Namespace) {
	_ = m.resourceQueue.Add(context.Background(), event{ns: ns, action: nsSet})
}

// NamespaceRemove to remove the namespace
func (m *HealthMonitor) NamespaceRemove(ns *corev1.Namespace) {
	_ = m.resourceQueue.Add(context.Background(), event{ns: ns, action: nsRemove})
}

// InstrumentationSet to set the instrumentation
func (m *HealthMonitor) InstrumentationSet(instrumentation *current.Instrumentation) {
	_ = m.resourceQueue.Add(context.Background(), event{inst: instrumentation, action: instSet})
}

// InstrumentationRemove to remove the instrumentation
func (m *HealthMonitor) InstrumentationRemove(instrumentation *current.Instrumentation) {
	_ = m.resourceQueue.Add(context.Background(), event{inst: instrumentation, action: instRemove})
}
