package util

import (
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestGetContainerByNameFromPod(t *testing.T) {

}

func TestSetPodLabel(t *testing.T) {
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
			SetPodLabel(test.pod, "foo", "bar")
			if diff := cmp.Diff(test.expectedPod, test.pod); diff != "" {
				assert.Fail(t, diff)
			}
		})
	}
}
