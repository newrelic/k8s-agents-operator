package instrumentation

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/apm"
)

var _ apm.Injector = (*ErrorInjector)(nil)

type ErrorInjector struct {
	err error
}

func (ei *ErrorInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	return pod, ei.err
}

func (ei *ErrorInjector) Language() string {
	return "error"
}

func (ei *ErrorInjector) ConfigureLogger(logger logr.Logger) {}

func (ei *ErrorInjector) ConfigureClient(client client.Client) {}

var _ apm.Injector = (*PanicInjector)(nil)

type PanicInjector struct {
	injectAttempted bool
}

func (pi *PanicInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	pi.injectAttempted = true
	var a *int
	var b int
	//nolint:all
	*a = b // nil pointer panic
	return corev1.Pod{}, nil
}

func (pi *PanicInjector) Language() string {
	return "panic"
}

func (pi *PanicInjector) ConfigureLogger(logger logr.Logger) {}

func (pi *PanicInjector) ConfigureClient(client client.Client) {}

var _ apm.Injector = (*AnnotationInjector)(nil)

type AnnotationInjector struct {
	lang string
}

func (ai *AnnotationInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations["injected-"+ai.lang] = "true"
	return pod, nil
}

func (ai *AnnotationInjector) Language() string {
	return ai.lang
}

func (ai *AnnotationInjector) ConfigureLogger(logger logr.Logger) {}

func (ai *AnnotationInjector) ConfigureClient(client client.Client) {}

func TestNewrelicSdkInjector_Inject(t *testing.T) {
	vtrue, vzero := true, int64(0)
	_, _ = vtrue, vzero
	logger := logr.Discard()
	tests := []struct {
		name          string
		langInsts     []*v1alpha2.Instrumentation
		ns            corev1.Namespace
		pod           corev1.Pod
		containerName string
		expectedPod   corev1.Pod
	}{
		{
			name: "empty",
		},
		{
			name:      "none",
			langInsts: []*v1alpha2.Instrumentation{},
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
		},
		{
			name: "inject just a",
			langInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "a"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"injected-a": "true"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
		},
		{
			name: "inject just b",
			langInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "b"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"injected-b": "true"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
		},
		{
			name: "inject a and b",
			langInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "a"}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "b"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"injected-a": "true", "injected-b": "true"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
		},
		{
			name: "inject has an error, pod should not be modified by that specific injector",
			langInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "error"}}},
			},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pod-name"}}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			injectorRegistry := apm.NewInjectorRegistry()
			apmInjectors := []apm.Injector{
				&AnnotationInjector{lang: "a"},
				&AnnotationInjector{lang: "b"},
				&ErrorInjector{err: fmt.Errorf("some error")},
			}
			for _, apmInjector := range apmInjectors {
				injectorRegistry.MustRegister(apmInjector)
			}
			defaulter := InstrumentationDefaulter{Logger: logger}
			for _, langInst := range test.langInsts {
				_ = defaulter.Default(ctx, langInst)
			}
			injector := NewNewrelicSdkInjector(logger, k8sClient, injectorRegistry)
			pod := injector.Inject(ctx, test.langInsts, test.ns, test.pod)
			if diff := cmp.Diff(test.expectedPod, pod); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestNewrelicSdkInjector_Inject_WithPanic(t *testing.T) {
	ctx := context.Background()
	var logger = logr.Discard()
	injectorRegistry := apm.NewInjectorRegistry()
	pi := &PanicInjector{}
	injectorRegistry.MustRegister(pi)
	injector := NewNewrelicSdkInjector(logger, k8sClient, injectorRegistry)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("failed to handle panic")
			}
		}()
		_ = injector.Inject(ctx, []*v1alpha2.Instrumentation{{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "panic", Image: "panic"}}}}, corev1.Namespace{}, corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "panic", Image: "panic"}}}})
	}()

	if !pi.injectAttempted {
		t.Fatalf("failed to trigger an injected panic")
	}
}
