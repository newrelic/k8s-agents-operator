package controller

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

type TestHealthMonitorForInstrumentation struct {
	set    func(instrumentation *current.Instrumentation)
	remove func(instrumentation *current.Instrumentation)
}

func (t *TestHealthMonitorForInstrumentation) InstrumentationSet(instrumentation *current.Instrumentation) {
	t.set(instrumentation)
}
func (t *TestHealthMonitorForInstrumentation) InstrumentationRemove(instrumentation *current.Instrumentation) {
	t.remove(instrumentation)
}

func TestReconcileInstrumentation(t *testing.T) {
	ctx := context.Background()
	ir := InstrumentationReconciler{
		Client:            k8sClient,
		operatorNamespace: "default",
		healthMonitor:     &TestHealthMonitorForInstrumentation{set: func(instrumentation *current.Instrumentation) {}, remove: func(instrumentation *current.Instrumentation) {}},
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
