package instrumentation

import (
	"context"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
	"github.com/newrelic/k8s-agents-operator/src/apm"
)

var _ webhook.CustomValidator = (*InstrumentationValidator)(nil)

type InstrumentationValidator struct {
	Logger            logr.Logger
	InjectorRegistery *apm.InjectorRegistery
}

func (r *InstrumentationValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	r.Logger.V(1).Info("validate_create", "name", obj.(*v1alpha2.Instrumentation).Name)
	acceptableLangs := r.InjectorRegistery.GetInjectors().Names()
	agentLang := obj.(*v1alpha2.Instrumentation).Spec.Agent.Language
	if !slices.Contains(acceptableLangs, agentLang) {
		return nil, fmt.Errorf("instrumentation agent language %q must be one of the accepted languages (%s)", agentLang, strings.Join(acceptableLangs, ", "))
	}
	return obj.(*v1alpha2.Instrumentation).ValidateCreate()
}

func (r *InstrumentationValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	r.Logger.V(1).Info("validate_update", "name", newObj.(*v1alpha2.Instrumentation).Name)
	return newObj.(*v1alpha2.Instrumentation).ValidateUpdate(oldObj)
}

func (r *InstrumentationValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	r.Logger.V(1).Info("validate_delete", "name", obj.(*v1alpha2.Instrumentation).Name)
	return obj.(*v1alpha2.Instrumentation).ValidateDelete()
}
