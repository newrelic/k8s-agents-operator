package instrumentation

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/apm"
)

var _ apm.ContainerInjector = (*ErrorInjector)(nil)

type ErrorInjector struct {
	err error
}

func (ei *ErrorInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	return pod, ei.err
}

func (ei *ErrorInjector) Language() string {
	return "error"
}

func (ei *ErrorInjector) Accepts(inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) bool {
	return inst.Spec.Agent.Language == ei.Language()
}

func (ei *ErrorInjector) ConfigureClient(client client.Client) {}

var _ apm.ContainerInjector = (*AnnotationInjector)(nil)

type AnnotationInjector struct {
	lang string
}

func (ai *AnnotationInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations["injected-"+ai.lang] = "true"
	return pod, nil
}

func (ai *AnnotationInjector) Language() string {
	return ai.lang
}

func (ai *AnnotationInjector) Accepts(inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) bool {
	return inst.Spec.Agent.Language == ai.Language()
}

func (ai *AnnotationInjector) ConfigureClient(client client.Client) {}

var (
	_ apm.ContainerInjector = (*ContainerInjector)(nil)
)

type ContainerInjector struct {
	lang string
}

func (i *ContainerInjector) InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error) {
	var container *corev1.Container
	for j, containerItem := range pod.Spec.Containers {
		if containerItem.Name == containerName {
			container = &pod.Spec.Containers[j]
		}
	}
	if container == nil {
		return pod, nil
	}
	container.Env = append(container.Env, corev1.EnvVar{
		Name: "injected", Value: "true",
	})
	return pod, nil
}

func (i *ContainerInjector) Language() string {
	return i.lang
}

func (i *ContainerInjector) Accepts(inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) bool {
	return inst.Spec.Agent.Language == i.Language()
}

func (ai *ContainerInjector) ConfigureClient(client client.Client) {}

func TestNewrelicSdkInjector_Inject(t *testing.T) {
	vtrue, vzero := true, int64(0)
	_, _ = vtrue, vzero
	tests := []struct {
		name          string
		langInsts     []*current.Instrumentation
		ns            corev1.Namespace
		pod           corev1.Pod
		containerName string
		expectedPod   corev1.Pod
		useNewMethod  bool
	}{
		{
			name: "empty",
		},
		{
			name:      "none",
			langInsts: []*current.Instrumentation{},
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "nothing",
				},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "nothing",
				},
			}}},
			containerName: "nothing",
		},
		{
			name: "inject just a",
			langInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "a"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"injected-a": "true"},
					Labels:      map[string]string{"newrelic-k8s-agents-operator-version": ""},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			containerName: "pod-name",
		},
		{
			name: "inject just b",
			langInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "b"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"injected-b": "true"},
					Labels:      map[string]string{"newrelic-k8s-agents-operator-version": ""},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			containerName: "pod-name",
		},
		{
			name: "inject a and b",
			langInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "a"}}},
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "b"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"injected-a": "true", "injected-b": "true"},
					Labels:      map[string]string{"newrelic-k8s-agents-operator-version": ""},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			containerName: "pod-name",
		},
		{
			name: "inject 1st container",
			langInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "c"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-1"}, {Name: "container-2"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"newrelic-k8s-agents-operator-version": ""},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-1", Env: []corev1.EnvVar{{Name: "injected", Value: "true"}}}, {Name: "container-2"}}},
			},
			containerName: "container-1",
		},
		{
			name: "inject has an error, pod should not be modified by that specific injector",
			langInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "error"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			containerName: "pod-name",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			injectorRegistry := apm.NewInjectorRegistry()
			apmInjectors := []apm.ContainerInjector{
				&AnnotationInjector{lang: "a"},
				&AnnotationInjector{lang: "b"},
				&ContainerInjector{lang: "c"},
				&ErrorInjector{err: fmt.Errorf("some error")},
			}
			for _, apmInjector := range apmInjectors {
				injectorRegistry.MustRegister(apmInjector)
			}
			defaulter := current.InstrumentationDefaulter{}
			for _, langInst := range test.langInsts {
				_ = defaulter.Default(ctx, langInst)
			}
			injector := NewNewrelicSdkInjector(k8sClient, injectorRegistry)
			pod := injector.InjectContainers(ctx, map[string][]*current.Instrumentation{test.containerName: test.langInsts}, test.ns, test.pod)
			if diff := cmp.Diff(test.expectedPod, pod); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestAssignHealthPorts(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name                   string
		matchedInstrumentation map[string][]*current.Instrumentation
		pod                    corev1.Pod
		expectedMap            map[string]int
	}{
		{
			name: "nothing reserved",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 0},
							},
						},
						{
							Name: "b",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 0},
							},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image"}}},
				},
				"b": {
					{Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image"}}},
				},
			},
			expectedMap: map[string]int{
				"a": 6194,
				"b": 6195,
			},
		},
		{
			name: "nothing reserved, blank values",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 0},
							},
						},
						{
							Name: "b",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 0},
							},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image", Env: []corev1.EnvVar{{Name: "NEW_RELIC_SIDECAR_LISTEN_PORT", Value: ""}}}}},
				},
				"b": {
					{Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image", Env: []corev1.EnvVar{{Name: "NEW_RELIC_SIDECAR_LISTEN_PORT", Value: ""}}}}},
				},
			},
			expectedMap: map[string]int{
				"a": 6194,
				"b": 6195,
			},
		},
		{
			name: "default port taken",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6194},
							},
						},
						{
							Name: "b",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 0},
							},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image"}}},
				},
				"b": {
					{Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image"}}},
				},
			},
			expectedMap: map[string]int{
				"a": 6195,
				"b": 6196,
			},
		},
		{
			name: "both ports taken, 1 statically assigned",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6194},
							},
						},
						{
							Name: "b",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6195},
							},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{
						Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image", Env: []corev1.EnvVar{{Name: "NEW_RELIC_SIDECAR_LISTEN_PORT", Value: "6197"}}}},
					},
				},
				"b": {
					{
						Spec: current.InstrumentationSpec{HealthAgent: current.HealthAgent{Image: "health-image"}},
					},
				},
			},
			expectedMap: map[string]int{
				"a": 6197,
				"b": 6196,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualMap := assignHealthPorts(ctx, tt.matchedInstrumentation, tt.pod)
			if diff := cmp.Diff(tt.expectedMap, actualMap); diff != "" {
				t.Errorf("actualMap differs from expected:\n%s", diff)
			}
		})
	}
}

func TestAssignHealthVolumes(t *testing.T) {
	tests := []struct {
		name                   string
		matchedInstrumentation map[string][]*current.Instrumentation
		pod                    corev1.Pod
		expectedMap            map[string]string
	}{
		{
			name: "all empty values, should be default",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Env:  []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
						{
							Name: "b",
							Env:  []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
					}},
				},
				"b": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
					}},
				},
			},
			expectedMap: map[string]string{
				"a": "file:///nri-health--a",
				"b": "file:///nri-health--b",
			},
		},
		{
			name: "already set on pods, should whatever is set on pod if it doesn't conflict",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Env:  []corev1.EnvVar{{Name: apmHealthLocation, Value: "/a"}},
						},
						{
							Name: "b",
							Env:  []corev1.EnvVar{{Name: apmHealthLocation, Value: "/b"}},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
					}},
				},
				"b": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: apmHealthLocation, Value: ""}},
						},
					}},
				},
			},
			expectedMap: map[string]string{
				"a": "/a",
				"b": "/b",
			},
		},
		{
			name: "use settings from agent for pod",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Env:  []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
						{
							Name: "b",
							Env:  []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: "/a"}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
					}},
				},
				"b": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: "/b"}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
					}},
				},
			},
			expectedMap: map[string]string{
				"a": "/a",
				"b": "/b",
			},
		},
		{
			name: "use settings from health agent for pod",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
							Env:  []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
						{
							Name: "b",
							Env:  []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
					},
				},
			},
			matchedInstrumentation: map[string][]*current.Instrumentation{
				"a": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: "/a"}},
						},
					}},
				},
				"b": {
					{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Image: "agent-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: ""}},
						},
						HealthAgent: current.HealthAgent{
							Image: "health-image",
							Env:   []corev1.EnvVar{{Name: "NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION", Value: "/b"}},
						},
					}},
				},
			},
			expectedMap: map[string]string{
				"a": "/a",
				"b": "/b",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualMap, _ := assignHealthVolumes(tt.matchedInstrumentation, tt.pod)
			if diff := cmp.Diff(tt.expectedMap, actualMap); diff != "" {
				t.Errorf("actualMap differs from expected:\n%s", diff)
			}
		})
	}
}
