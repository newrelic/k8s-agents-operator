package apm

import (
	"github.com/newrelic/k8s-agents-operator/api/current"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBaseInjector_ConfigureClient(t *testing.T) {

}

func TestBaseInjector_ConfigureLogger(t *testing.T) {

}

func TestInjectorRegistery_Register(t *testing.T) {

}

func TestInjectorRegistery_MustRegister(t *testing.T) {

}

func TestInjectors_Names(t *testing.T) {

}

func TestApplyLabel(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		expectedPod    *corev1.Pod
		expectedErrStr string
	}{
		{
			name: "a container with labels added",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_LABELS", Value: "app:java-injected"},
						}}}}},

			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_LABELS", Value: "app:java-injected"},
						},
					}},
				}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualPod := applyLabelToPod(test.pod, "foo", "bar")

			if diff := cmp.Diff(test.expectedPod, actualPod); diff != "" {
				assert.Fail(t, diff)
			}
		})
	}
}

func TestEncodeDecodeAttributes(t *testing.T) {
	var diff string
	diff = cmp.Diff(map[string]string{"a": "b", "c": "d", "e": "f"}, decodeAttributes("a:b;c:d;e:f", ";", ":"))
	if diff != "" {
		assert.Fail(t, diff)
	}
	diff = cmp.Diff("a:b;c:d;e:f", encodeAttributes(map[string]string{"a": "b", "c": "d", "e": "f"}, ";", ":"))
	if diff != "" {
		assert.Fail(t, diff)
	}
}

func TestSetContainerEnvFromInst(t *testing.T) {
	container := corev1.Container{}
	expectedContainer := corev1.Container{Env: []corev1.EnvVar{{Name: "A", Value: "B"}}}
	setContainerEnvFromInst(&container, current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Env: []corev1.EnvVar{{Name: "A", Value: "B"}}}}})
	if diff := cmp.Diff(expectedContainer, container); diff != "" {
		assert.Fail(t, diff)
	}
}
