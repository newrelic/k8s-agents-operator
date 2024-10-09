package instrumentation

import (
	"context"
	"fmt"
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

func (r *InstrumentationValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	r.Logger.V(1).Info("validate_create", "name", obj.(*v1alpha2.Instrumentation).Name)
	injectors := r.InjectorRegistery.GetInjectors()
	acceptableLangs := injectors.Names()
	inst := obj.(*v1alpha2.Instrumentation)
	agentLang := inst.Spec.Agent.Language
	if !slices.Contains(acceptableLangs, agentLang) {
		return fmt.Errorf("instrumentation agent language %q must be one of the accepted languages (%s)", agentLang, strings.Join(acceptableLangs, ", "))
	}
	for _, injector := range injectors {
		if agentLang != injector.Language() {
			continue
		}
		injectionValidator, ok := injector.(apm.AgentValidator)
		if !ok {
			continue
		}
		if err := injectionValidator.ValidateAgent(ctx, inst.Spec.Agent); err != nil {
			return err
		}
	}
	return obj.(*v1alpha2.Instrumentation).ValidateCreate()
}

func (r *InstrumentationValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) error {
	r.Logger.V(1).Info("validate_update", "name", newObj.(*v1alpha2.Instrumentation).Name)
	return newObj.(*v1alpha2.Instrumentation).ValidateUpdate(oldObj)
}

func (r *InstrumentationValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	r.Logger.V(1).Info("validate_delete", "name", obj.(*v1alpha2.Instrumentation).Name)
	return obj.(*v1alpha2.Instrumentation).ValidateDelete()
}
