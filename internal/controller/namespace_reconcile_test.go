package controller

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

type TestHealthMonitorForNamespace struct {
	set    func(ns *corev1.Namespace)
	remove func(ns *corev1.Namespace)
}

func (t *TestHealthMonitorForNamespace) NamespaceSet(ns *corev1.Namespace) {
	t.set(ns)
}
func (t *TestHealthMonitorForNamespace) NamespaceRemove(ns *corev1.Namespace) {
	t.remove(ns)
}

func TestReconcileNamespace(t *testing.T) {
	ctx := context.Background()
	ir := NamespaceReconciler{
		Client:        k8sClient,
		healthMonitor: &TestHealthMonitorForNamespace{set: func(ns *corev1.Namespace) {}, remove: func(ns *corev1.Namespace) {}},
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
			name:              "unacceptable resource",
			resourceName:      "nothing",
			resourceNamespace: "kube-system",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ir.Reconcile(ctx, controllerruntime.Request{NamespacedName: types.NamespacedName{Name: tt.resourceName, Namespace: tt.resourceNamespace}})
			if err != nil && tt.expectedErr != nil {
				if !reflect.DeepEqual(err, tt.expectedErr) {
					t.Errorf("got unexpected err: %v, expected: %v", res, tt.expectedErr)
				}
			} else if err == nil && tt.expectedErr != nil {
				if !reflect.DeepEqual(err, tt.expectedErr) {
					t.Errorf("expected err: %v", tt.expectedErr)
				}
			} else if err != nil && tt.expectedErr == nil {
				if !reflect.DeepEqual(err, tt.expectedErr) {
					t.Errorf("got unexpected err: %v", res)
				}
			} else {
				if !reflect.DeepEqual(res, tt.expectedResult) {
					t.Errorf("got unexpected result: %v, expected: %v", res, tt.expectedResult)
				}
			}
		})
	}
}
