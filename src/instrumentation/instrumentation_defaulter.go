package instrumentation

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
)

var _ webhook.CustomDefaulter = (*InstrumentationDefaulter)(nil)

type InstrumentationDefaulter struct {
	Logger logr.Logger
}

func (r *InstrumentationDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	r.Logger.V(1).Info("default", "name", obj.(*v1alpha2.Instrumentation).Name)
	obj.(*v1alpha2.Instrumentation).Default()
	return nil
}
