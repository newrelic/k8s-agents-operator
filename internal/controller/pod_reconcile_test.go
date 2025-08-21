package controller

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

type TestHealthMonitorForPod struct {
	set    func(pod *corev1.Pod)
	remove func(pod *corev1.Pod)
}

func (t *TestHealthMonitorForPod) PodSet(pod *corev1.Pod) {
	t.set(pod)
}
func (t *TestHealthMonitorForPod) PodRemove(pod *corev1.Pod) {
	t.remove(pod)
}

func TestReconcilePod(t *testing.T) {
	ctx := context.Background()
	ir := PodReconciler{
		Client:            k8sClient,
		operatorNamespace: "default",
		healthMonitor:     &TestHealthMonitorForPod{set: func(pod *corev1.Pod) {}, remove: func(pod *corev1.Pod) {}},
	}

	tests := []struct {
		name              string
		resourceName      string
		resourceNamespace string
		expectedResult    controllerruntime.Result
		expectedErr       error
	}{
		{
			name:              "non existent resource",
			resourceName:      "nothing",
			resourceNamespace: "default",
		},
		{
			name:              "not in operator namespace",
			resourceName:      "nothing",
			resourceNamespace: "other",
		},
		{
			name:              "unacceptable resource",
			resourceName:      "nothing",
			resourceNamespace: "kube-system",
		},
		{
			name:              "unacceptable resource 2",
			resourceName:      "nothing",
			resourceNamespace: "newrelic",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ir.Reconcile(ctx, controllerruntime.Request{NamespacedName: types.NamespacedName{Name: tt.resourceName, Namespace: tt.resourceNamespace}})

			if err != nil && tt.expectedErr != nil && !reflect.DeepEqual(err, tt.expectedErr) {
				t.Errorf("got unexpected err: %v, expected: %v", res, tt.expectedErr)
			} else if err == nil && tt.expectedErr != nil {
				t.Errorf("expected err: %v", tt.expectedErr)
			} else if err != nil && tt.expectedErr == nil {
				t.Errorf("got unexpected err: %v", res)
			} else if !reflect.DeepEqual(res, tt.expectedResult) {
				t.Errorf("got unexpected result: %v, expected: %v", res, tt.expectedResult)
			}
		})
	}
}
