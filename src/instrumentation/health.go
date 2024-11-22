package instrumentation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/newrelic/k8s-agents-operator/src/apm"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

const healthSidecarContainerName = "newrelic-apm-health"

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

type InstrumentationMetric struct {
	instrumentationID string
	instrumentation   *v1alpha2.Instrumentation
	podMetrics        []*PodMetric
	doneCh            chan struct{}
	podsMatching      int64
	podsInjected      int64
	podsNotReady      int64
	podsOutdated      int64
	podsHealthy       int64
	podsUnhealthy     int64
	unhealthyPods     []v1alpha2.UnhealthyPodError
}

func (im *InstrumentationMetric) IsDiff() bool {
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

func (im *InstrumentationMetric) SyncStatus() {
	im.instrumentation.Status.PodsOutdated = im.podsOutdated
	im.instrumentation.Status.PodsInjected = im.podsInjected
	im.instrumentation.Status.PodsMatching = im.podsMatching
	im.instrumentation.Status.PodsOutdated = im.podsOutdated
	im.instrumentation.Status.PodsNotReady = im.podsNotReady
	im.instrumentation.Status.PodsHealthy = im.podsHealthy
	im.instrumentation.Status.PodsUnhealthy = im.podsUnhealthy
	im.instrumentation.Status.UnhealthyPodsErrors = im.unhealthyPods
}

type PodMetric struct {
	pod              *corev1.Pod
	podID            string
	needsHealthCheck bool
	health           Health
	doneCh           chan struct{}
}

// Health opamp format; @todo: update this
type Health struct {
	Healthy    bool   `json:"healthy"`
	Status     string `json:"status"`
	StartTime  string `json:"start_time_unix_nano"`
	StatusTime string `json:"status_time_unix_nano"`
	AgentRunID string `json:"agent_run_id"`
	LastError  string `json:"last_error"`
}

type HealthMonitor struct {
	client.Client
	Scheme           *runtime.Scheme
	instrumentations map[string]*v1alpha2.Instrumentation
	pods             map[string]*corev1.Pod
	namespaces       map[string]*corev1.Namespace
	events           chan event
	closeCh          chan struct{}
	safeCloseCh      chan struct{}
	closeOnce        *sync.Once
	healthApi        *healthCheckApi

	podWorkerCfg                                  MetricsWorker
	instrumentationWorkerCfg                      MetricsWorker
	podMetricsBatchCh                             chan []*PodMetric
	instrumentationMetricsBatchCh                 chan []*InstrumentationMetric
	waitForInstrumentationsMetricsCh              chan []*InstrumentationMetric
	waitForPodMetricsForInstrumentationsMetricsCh chan []*InstrumentationMetric
}

type MetricsWorker struct {
	minWorkers     int
	maxWorkers     int
	scaleUpDelay   time.Duration
	scaleDownDelay time.Duration
}

func NewHealthMonitor(ctx context.Context, client client.Client) *HealthMonitor {
	healthApi := &healthCheckApi{httpClient: &http.Client{}}
	m := &HealthMonitor{
		healthApi:        healthApi,
		Client:           client,
		instrumentations: make(map[string]*v1alpha2.Instrumentation),
		pods:             make(map[string]*corev1.Pod),
		namespaces:       make(map[string]*corev1.Namespace),
		closeCh:          make(chan struct{}),
		safeCloseCh:      make(chan struct{}),
		closeOnce:        &sync.Once{},
		events:           make(chan event, 100),

		podWorkerCfg: MetricsWorker{
			minWorkers:     50,
			maxWorkers:     200,
			scaleUpDelay:   5 * time.Second,
			scaleDownDelay: 1 * time.Second,
		},
		instrumentationWorkerCfg: MetricsWorker{
			minWorkers:     2,
			maxWorkers:     10,
			scaleUpDelay:   15 * time.Second,
			scaleDownDelay: 1 * time.Second,
		},
		podMetricsBatchCh:                make(chan []*PodMetric),
		instrumentationMetricsBatchCh:    make(chan []*InstrumentationMetric),
		waitForInstrumentationsMetricsCh: make(chan []*InstrumentationMetric),
	}
	m.start(ctx)
	return m
}

func (m *HealthMonitor) Stop(ctx context.Context) error {
	m.closeOnce.Do(func() {
		close(m.safeCloseCh)
	})
	select {
	case <-m.closeCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *HealthMonitor) PodSet(pod *corev1.Pod) {
	m.events <- event{pod: pod, action: podSet}
}

func (m *HealthMonitor) PodRemove(pod *corev1.Pod) {
	m.events <- event{pod: pod, action: podRemove}
}

func (m *HealthMonitor) NamespaceSet(ns *corev1.Namespace) {
	m.events <- event{ns: ns, action: nsSet}
}

func (m *HealthMonitor) NamespaceRemove(ns *corev1.Namespace) {
	m.events <- event{ns: ns, action: nsRemove}
}

func (m *HealthMonitor) InstrumentationSet(instrumentation *v1alpha2.Instrumentation) {
	m.events <- event{inst: instrumentation, action: instSet}
}

func (m *HealthMonitor) InstrumentationRemove(instrumentation *v1alpha2.Instrumentation) {
	m.events <- event{inst: instrumentation, action: instRemove}
}

func (m *HealthMonitor) OnNamespaceSet(ev event) {
	m.namespaces[ev.ns.Name] = ev.ns
}

func (m *HealthMonitor) OnNamespaceRemove(ev event) {
	if ns, ok := m.namespaces[ev.ns.Name]; ok {
		ns.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
	delete(m.namespaces, ev.ns.Name)
}

func (m *HealthMonitor) OnPodSet(ev event) {
	id := types.NamespacedName{Namespace: ev.pod.Namespace, Name: ev.pod.Name}.String()
	m.pods[id] = ev.pod
}

func (m *HealthMonitor) OnPodRemove(ev event) {
	id := types.NamespacedName{Namespace: ev.pod.Namespace, Name: ev.pod.Name}.String()
	if pod, ok := m.pods[id]; ok {
		pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
	delete(m.pods, id)
}

func (m *HealthMonitor) OnInstrumentationSet(ev event) {
	id := types.NamespacedName{Namespace: ev.inst.Namespace, Name: ev.inst.Name}.String()
	m.instrumentations[id] = ev.inst
}

func (m *HealthMonitor) OnInstrumentationRemove(ev event) {
	id := types.NamespacedName{Namespace: ev.inst.Namespace, Name: ev.inst.Name}.String()
	if inst, ok := m.instrumentations[id]; ok {
		inst.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
	delete(m.instrumentations, id)
}

func (m *HealthMonitor) StartInstrumentationMetricsCalculator(ctx context.Context, instrumentationMetricsBatchCh chan []*InstrumentationMetric) {
	logger := log.FromContext(ctx)
	instrumentationMetricsCh := make(chan *InstrumentationMetric)
	l := m.instrumentationWorkerCfg.minWorkers
	workers := int64(l)
	for i := 0; i < l; i++ {
		go m.InstrumentationMetricsCalculationWorker(ctx, false, instrumentationMetricsCh)
		logger.Info("start init worker: instrumentation")
	}

	for {
		select {
		case instrumentationMetricsBatch := <-instrumentationMetricsBatchCh:
			for _, instrumentationMetric := range instrumentationMetricsBatch {
				select {
				case <-time.After(m.instrumentationWorkerCfg.scaleUpDelay):
					count := atomic.LoadInt64(&workers)
					if int(count) >= m.instrumentationWorkerCfg.maxWorkers {
						instrumentationMetricsCh <- instrumentationMetric
						continue
					}
					logger.Info("scale up: instrumentation")
					atomic.AddInt64(&workers, 1)
					go func() {
						defer atomic.AddInt64(&workers, -1)
						m.InstrumentationMetricsCalculationWorker(ctx, true, instrumentationMetricsCh)
					}()
					instrumentationMetricsCh <- instrumentationMetric
				case instrumentationMetricsCh <- instrumentationMetric:
				}
			}
		}
	}
}

func (m *HealthMonitor) StartInstrumentationMetricsCalculatorW(ctx context.Context, instrumentationMetricsBatchCh chan []*InstrumentationMetric) {
	logger := log.FromContext(ctx)
	instrumentationMetricsPendingCh := make(chan *InstrumentationMetric)
	instrumentationMetricsReadyCh := make(chan *InstrumentationMetric)
	scaleRequestCh := make(chan int)

	l := 50
	for i := 0; i < l; i++ {
		go m.WaitForPodMetricsForInstrumentationMetrics(ctx, scaleRequestCh, instrumentationMetricsPendingCh, instrumentationMetricsReadyCh)
		logger.Info("start init worker: instrumentation waiter")
	}

	workers := 0
	go func() {
		scaleDownCh := make(chan struct{})
		defer close(scaleDownCh)
		lastScaleUp := time.Time{}
		lastScaleDown := time.Time{}
		id := 1
		for scale := range scaleRequestCh {
			if scale == -1 {
				if workers <= m.instrumentationWorkerCfg.minWorkers {
					continue
				}
				if time.Since(lastScaleDown) < m.instrumentationWorkerCfg.scaleDownDelay {
					continue
				}
				scaleDownCh <- struct{}{}
				workers--
				lastScaleDown = time.Now()
				logger.Info("stop worker: instrumentation", "workers", workers)
			}
			if scale == 1 {
				if workers >= m.instrumentationWorkerCfg.maxWorkers {
					continue
				}
				if time.Since(lastScaleUp) < m.instrumentationWorkerCfg.scaleUpDelay {
					continue
				}
				workers++
				lastScaleUp = time.Now()
				go m.InstrumentationMetricsCalculationWorkerW(ctx, scaleRequestCh, instrumentationMetricsReadyCh)
				logger.Info("start worker: instrumentation", "workers", workers, "id", id)
				id++
			}
		}
	}()
	l = m.instrumentationWorkerCfg.minWorkers
	for i := 0; i < l; i++ {
		go m.InstrumentationMetricsCalculationWorkerW(ctx, scaleRequestCh, instrumentationMetricsReadyCh)
		workers++
	}

	for {
		select {
		case instrumentationMetricsBatch := <-instrumentationMetricsBatchCh:
			for _, instrumentationMetric := range instrumentationMetricsBatch {
				select {
				case instrumentationMetricsPendingCh <- instrumentationMetric:
				}
			}
		}
	}
}

func (m *HealthMonitor) WaitForPodMetricsForInstrumentationMetrics(ctx context.Context, scaleRequestCh chan int, instrumentationMetricsPendingCh chan *InstrumentationMetric, instrumentationMetricsReadyCh chan *InstrumentationMetric) {
	for instrumentationMetric := range instrumentationMetricsPendingCh {
		for _, podMetric := range instrumentationMetric.podMetrics {
			if podMetric.doneCh != nil {
				<-podMetric.doneCh
			}
		}
		select {
		case <-time.After(m.instrumentationWorkerCfg.scaleUpDelay):
			scaleRequestCh <- 1
		case instrumentationMetricsReadyCh <- instrumentationMetric:
		}

	}
}

func (m *HealthMonitor) InstrumentationMetricsCalculationWorkerW(ctx context.Context, scaleRequestCh chan int, instrumentationMetricsCh chan *InstrumentationMetric) {
	for {
		select {
		case <-time.After(m.instrumentationWorkerCfg.scaleDownDelay):
			scaleRequestCh <- -1
		case instrumentationMetric := <-instrumentationMetricsCh:
			m.UpdateInstrumentation(ctx, instrumentationMetric)
		}
	}
}

func (m *HealthMonitor) InstrumentationMetricsCalculationWorker(ctx context.Context, isEphemeral bool, instrumentationMetricsCh chan *InstrumentationMetric) {
	logger := log.FromContext(ctx)
	for {
		select {
		case <-time.After(m.instrumentationWorkerCfg.scaleDownDelay):
			if !isEphemeral {
				continue
			}
			//scale down
			logger.Info("scale down: instrumentation")
			return
		case instrumentationMetric := <-instrumentationMetricsCh:
			m.UpdateInstrumentation(ctx, instrumentationMetric)
		}
	}
}

const instrumentationVersionAnnotation = "newrelic.com/instrumentation-versions"

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

func (m *HealthMonitor) UpdateInstrumentation(ctx context.Context, instrumentationMetric *InstrumentationMetric) {
	logger := log.FromContext(ctx)
	for _, podMetric := range instrumentationMetric.podMetrics {
		// only wait for pod metrics that have a channel. these are the only ones waiting for a health check
		if podMetric.doneCh != nil {
			<-podMetric.doneCh
		}

		instrumentationMetric.podsMatching++
		if !m.isPodInstrumented(podMetric.pod) {
			continue
		}
		if m.isPodOutdated(podMetric.pod, instrumentationMetric.instrumentation) {
			instrumentationMetric.podsOutdated++
		}
		instrumentationMetric.podsInjected++
		if !m.isPodReady(podMetric.pod) {
			instrumentationMetric.podsNotReady++
			continue
		}

		if podMetric.health.Healthy {
			instrumentationMetric.podsHealthy++
		} else {
			instrumentationMetric.podsUnhealthy++
			instrumentationMetric.unhealthyPods = append(instrumentationMetric.unhealthyPods, v1alpha2.UnhealthyPodError{
				Pod:       podMetric.podID,
				LastError: podMetric.health.LastError,
			})
		}
	}

	if instrumentationMetric.IsDiff() {
		instrumentationMetric.SyncStatus()
		instrumentationMetric.instrumentation.Status.LastUpdated = metav1.Now()
		if err := m.Client.Status().Update(ctx, instrumentationMetric.instrumentation); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to update status for instrumentation")
			}
		}
		logger.Info("wrote status for instrumentation", "id", instrumentationMetric.instrumentationID)
	} else {
		logger.Info("no changes to status for instrumentation", "id", instrumentationMetric.instrumentationID)
	}

	close(instrumentationMetric.doneCh)
}

func (m *HealthMonitor) StartPodMetricsCalculator(ctx context.Context, podMetricsBatchCh chan []*PodMetric) {
	logger := log.FromContext(ctx)
	podMetricsCh := make(chan *PodMetric)
	l := m.podWorkerCfg.minWorkers
	workers := int64(l)
	for i := 0; i < l; i++ {
		go m.PodMetricsCalculationWorker(ctx, false, podMetricsCh)
		logger.Info("start init worker: pod")
	}
	for {
		select {
		case podMetrics := <-podMetricsBatchCh:
			for _, podMetric := range podMetrics {
				if !m.isPodInstrumented(podMetric.pod) {
					continue
				}
				if !m.isPodReady(podMetric.pod) {
					continue
				}
				podMetric.doneCh = make(chan struct{})
				select {
				case <-time.After(time.Second):
					count := atomic.LoadInt64(&workers)
					if int(count) >= m.instrumentationWorkerCfg.maxWorkers {
						podMetricsCh <- podMetric
						continue
					}
					logger.Info("scale up: pod")
					atomic.AddInt64(&workers, 1)
					go func() {
						defer atomic.AddInt64(&workers, -1)
						go m.PodMetricsCalculationWorker(ctx, true, podMetricsCh)
					}()
					podMetricsCh <- podMetric
				case podMetricsCh <- podMetric:
				}
			}
		}
	}
}

func (m *HealthMonitor) PodMetricsCalculationWorker(ctx context.Context, isEphemeral bool, podMetricsCh chan *PodMetric) {
	logger := log.FromContext(ctx)
	for {
		select {
		case <-time.After(time.Second):
			if !isEphemeral {
				continue
			}
			//scale down
			logger.Info("scale down: pod")
			return
		case podMetric := <-podMetricsCh:
			m.Check(ctx, podMetric)
		}
	}
}

func (m *HealthMonitor) StartWaitForInstrumentationMetricsDoneWorker(ctx context.Context, instrumentationMetricsBatchCh chan []*InstrumentationMetric) {
	for {
		select {
		case instrumentationMetrics := <-instrumentationMetricsBatchCh:
			for _, instrumentationMetric := range instrumentationMetrics {
				<-instrumentationMetric.doneCh
			}
			m.events <- event{action: healthChecksDone}
		}
	}
}

func (m *HealthMonitor) start(ctx context.Context) {
	go m.StartPodMetricsCalculator(ctx, m.podMetricsBatchCh)
	go m.StartInstrumentationMetricsCalculatorW(ctx, m.instrumentationMetricsBatchCh)
	go m.StartWaitForInstrumentationMetricsDoneWorker(ctx, m.waitForInstrumentationsMetricsCh)
	go m.StartEventLoop(ctx)
}

func (m *HealthMonitor) StartEventLoop(ctx context.Context) {
	logger := log.FromContext(ctx)
	tickInterval := time.Second * 15
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	defer close(m.closeCh)
	ticksToSkip := 0

	healthCheckStart := time.Time{}
	healthCheckIsActive := false

	for {
		select {
		case <-m.safeCloseCh:
			return
		case <-ctx.Done():
			return
		case _ = <-ticker.C:
			if ticksToSkip > 0 {
				ticksToSkip--
				continue
			}
			if healthCheckIsActive {
				continue
			}
			podMetrics := m.GetPodMetrics(ctx)
			if len(podMetrics) == 0 {
				logger.Info("nothing to check the health of.  No pods")
				continue
			}
			instrumentationMetrics := m.GetInstrumentationMetrics(ctx, podMetrics)
			if len(instrumentationMetrics) == 0 {
				logger.Info("nothing to report the health to.  No instrumentations")
				continue
			}

			logger.Info("starting the scheduling of health checks")
			healthCheckStart = time.Now()
			healthCheckIsActive = true

			m.podMetricsBatchCh <- podMetrics
			m.instrumentationMetricsBatchCh <- instrumentationMetrics
			m.waitForInstrumentationsMetricsCh <- instrumentationMetrics

			logger.Info("finished scheduling health checks")

		case ev := <-m.events:
			switch ev.action {
			case healthChecksDone:
				healthCheckIsActive = false
				totalTime := time.Since(healthCheckStart)

				// skip a tick (or more) if the time it takes exceeds our interval. round off the extra, otherwise we always skip at least 1
				ticksToSkip = int(totalTime / tickInterval)
				if ticksToSkip > 0 {
					logger.Info(fmt.Sprintf("Skipping %d ticks at a interval of %s", ticksToSkip, tickInterval.String()))
				}
			case nsSet:
				m.OnNamespaceSet(ev)
			case nsRemove:
				m.OnNamespaceRemove(ev)
			case podSet:
				m.OnPodSet(ev)
			case podRemove:
				m.OnPodRemove(ev)
			case instSet:
				m.OnInstrumentationSet(ev)
			case instRemove:
				m.OnInstrumentationRemove(ev)
			}
		}
	}
}

func (m *HealthMonitor) Check(ctx context.Context, podMetric *PodMetric) {
	logger := log.FromContext(ctx)
	logger.Info("checking health for pod", "pod", podMetric.podID)
	defer func() {
		logger.Info("closing pod metric", "pod", podMetric.podID)
		close(podMetric.doneCh)
	}()

	pod := podMetric.pod
	podHealthUrl, err := m.getHealthUrlFromPod(*pod)
	if err != nil {
		podMetric.health = Health{
			Healthy:   false,
			LastError: fmt.Sprintf("failed to identify health url > %s", err.Error()),
		}
		return
	}

	healthCtx, healthCtxCancel := context.WithTimeout(ctx, time.Second*15)
	defer healthCtxCancel()
	health, err := m.healthApi.GetHealth(healthCtx, podHealthUrl)
	if err != nil {
		podMetric.health = Health{
			Healthy:   false,
			LastError: fmt.Sprintf("failed while retrieving health > %s", err.Error()),
		}
		return
	}
	logger.Info("collected health for pod", "pod", podMetric.podID, "health", health)
	podMetric.health = health
}

func (m *HealthMonitor) GetPodMetrics(ctx context.Context) []*PodMetric {
	podMetrics := make([]*PodMetric, len(m.pods))
	i := 0
	for _, pod := range m.pods {
		podMetrics[i] = &PodMetric{pod: pod, podID: types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}.String()}
		i++
	}
	return podMetrics[0:i]
}

func (m *HealthMonitor) GetInstrumentationMetrics(ctx context.Context, podMetrics []*PodMetric) []*InstrumentationMetric {
	logger := log.FromContext(ctx)
	var instrumentationMetrics = make([]*InstrumentationMetric, len(m.instrumentations))
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

		var instPodMetrics []*PodMetric
		for _, podMetric := range podMetrics {
			ns, ok := m.namespaces[podMetric.pod.Namespace]
			if !ok {
				continue
			}
			if !namespaceSelector.Matches(labels.Set(ns.Labels)) {
				continue
			}
			if !podSelector.Matches(labels.Set(podMetric.pod.Labels)) {
				continue
			}
			instPodMetrics = append(instPodMetrics, podMetric)
			matchedPodMetricIDs[podMetric.podID] = struct{}{}
		}

		instrumentationMetrics[i] = &InstrumentationMetric{
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
		return "", nil
	}
	sidecar := sidecars[0]
	if len(sidecar.Ports) == 0 {
		// @todo: log error, we need a port for a health check
		return "", nil
	}
	if len(sidecar.Ports) > 1 {
		// @todo: log error, how do we know which is the health port?
		return "", nil
	}
	return fmt.Sprintf("http://%s:%d/healthz", pod.Status.PodIP, sidecar.Ports[0].ContainerPort), nil
}

func (m *HealthMonitor) isPodReady(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}

func (m *HealthMonitor) isPodInstrumented(pod *corev1.Pod) bool {
	if pod.Labels == nil {
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

type healthCheckApi struct {
	httpClient *http.Client
}

func (h *healthCheckApi) GetHealth(ctx context.Context, url string) (health Health, err error) {
	httpReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return health, fmt.Errorf("failed to create request > %w", err)
	}

	httpClient := &http.Client{}
	res, err := httpClient.Do(httpReq.WithContext(ctx))
	if err != nil {
		return health, fmt.Errorf("failed to send request > %w", err)
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return health, fmt.Errorf("failed to get expected response code of 200 > %w", err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return health, fmt.Errorf("failed to read response body > %w", err)
	}

	err = yaml.Unmarshal(body, &health)
	if err != nil {
		return health, fmt.Errorf("failed to parse response > %w", err)
	}
	return health, nil
}
