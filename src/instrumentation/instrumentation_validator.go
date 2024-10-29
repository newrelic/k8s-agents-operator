package instrumentation

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/apm"
)

const (
	envNewRelicPrefix = "NEW_RELIC_"
	envOtelPrefix     = "OTEL_"
)

var validEnvPrefixes = []string{envNewRelicPrefix, envOtelPrefix}
var validEnvPrefixesStr = strings.Join(validEnvPrefixes, ", ")

var _ webhook.CustomValidator = (*InstrumentationValidator)(nil)

// InstrumentationValidator is used to validate instrumentations
type InstrumentationValidator struct {
	Logger            logr.Logger
	InjectorRegistery *apm.InjectorRegistery
}

// ValidateCreate to validate the creation operation
func (r *InstrumentationValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	inst := obj.(*v1alpha2.Instrumentation)
	r.Logger.V(1).Info("validate_create", "name", inst.Name)
	return r.validate(inst)
}

// ValidateUpdate to validate the update operation
func (r *InstrumentationValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	inst := newObj.(*v1alpha2.Instrumentation)
	r.Logger.V(1).Info("validate_update", "name", inst.Name)
	return r.validate(inst)
}

// ValidateDelete to validate the deletion operation
func (r *InstrumentationValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	inst := obj.(*v1alpha2.Instrumentation)
	r.Logger.V(1).Info("validate_delete", "name", inst.Name)
	return r.validate(inst)
}

// validate to validate all the fields
func (r *InstrumentationValidator) validate(inst *v1alpha2.Instrumentation) (admission.Warnings, error) {
	acceptableLangs := r.InjectorRegistery.GetInjectors().Names()
	agentLang := inst.Spec.Agent.Language
	if !slices.Contains(acceptableLangs, agentLang) {
		return nil, fmt.Errorf("instrumentation agent language %q must be one of the accepted languages (%s)", agentLang, strings.Join(acceptableLangs, ", "))
	}

	if err := r.validateEnv(inst.Spec.Agent.Env); err != nil {
		return nil, err
	}

	if inst.Spec.Agent.IsEmpty() {
		return nil, fmt.Errorf("instrumentation %q agent is empty", inst.Name)
	}

	if _, err := metav1.LabelSelectorAsSelector(&inst.Spec.PodLabelSelector); err != nil {
		return nil, err
	}
	if _, err := metav1.LabelSelectorAsSelector(&inst.Spec.NamespaceLabelSelector); err != nil {
		return nil, err
	}

	return nil, nil
}

// validateEnv to validate the environment variables used all start with the required prefixes
func (r *InstrumentationValidator) validateEnv(envs []corev1.EnvVar) error {
	var invalidNames []string
	for _, env := range envs {
		var valid bool
		for _, validEnvPrefix := range validEnvPrefixes {
			if strings.HasPrefix(env.Name, validEnvPrefix) {
				valid = true
				break
			}
		}
		if !valid {
			invalidNames = append(invalidNames, env.Name)
		}
	}
	if len(invalidNames) > 0 {
		return fmt.Errorf("env name should start with %s; found these invalid names %s", validEnvPrefixesStr, strings.Join(invalidNames, ", "))
	}
	return nil
}
