package instrumentation

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
)

var _ webhook.CustomDefaulter = (*InstrumentationDefaulter)(nil)

// InstrumentationDefaulter is used to set defaults for instrumentation
type InstrumentationDefaulter struct {
	Logger logr.Logger
}

// Default to set the default values for Instrumentation
func (r *InstrumentationDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	inst := obj.(*v1alpha2.Instrumentation)
	r.Logger.V(1).Info("default", "name", inst.Name)
	if inst.Labels == nil {
		inst.Labels = map[string]string{}
	}
	if inst.Labels["app.kubernetes.io/managed-by"] == "" {
		inst.Labels["app.kubernetes.io/managed-by"] = "k8s-agents-operator"
	}
	if inst.Spec.LicenseKeySecret == "" {
		inst.Spec.LicenseKeySecret = "newrelic-key-secret"
	}
	return nil
}
