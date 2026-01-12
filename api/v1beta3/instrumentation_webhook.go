/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta3

import (
	"context"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func SetupWebhookWithManager(mgr ctrl.Manager, operatorNamespace string) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Instrumentation{}).
		WithValidator(NewInstrumentationValidator(operatorNamespace)).
		WithDefaulter(&InstrumentationDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-newrelic-com-v1beta3-instrumentation,mutating=true,failurePolicy=fail,sideEffects=None,groups=newrelic.com,resources=instrumentations,verbs=create;update,versions=v1beta3,name=minstrumentation-v1beta3.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = (*InstrumentationDefaulter)(nil)

// InstrumentationDefaulter is used to set defaults for instrumentation
type InstrumentationDefaulter struct {
}

// Default to set the default values for Instrumentation
func (r *InstrumentationDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	inst := obj.(*Instrumentation)
	log.FromContext(ctx).V(1).Info("Setting defaults for v1beta3.Instrumentation", "name", inst.GetName())
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

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:verbs=create;update,path=/validate-newrelic-com-v1beta3-instrumentation,mutating=false,failurePolicy=fail,groups=newrelic.com,resources=instrumentations,versions=v1beta3,name=vinstrumentationcreateupdate-v1beta3.kb.io,sideEffects=none,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=delete,path=/validate-newrelic-com-v1beta3-instrumentation,mutating=false,failurePolicy=ignore,groups=newrelic.com,resources=instrumentations,versions=v1beta3,name=vinstrumentationdelete-v1beta3.kb.io,sideEffects=none,admissionReviewVersions=v1

var validEnvPrefixes = []string{"NEW_RELIC_", "NEWRELIC_"}
var validEnvPrefixesStr = strings.Join(validEnvPrefixes, ", ")

var _ webhook.CustomValidator = (*InstrumentationValidator)(nil)

// +k8s:deepcopy-gen=false
// InstrumentationValidator is used to validate instrumentations
type InstrumentationSpecValidator func(instrumentation *Instrumentation) error

// +k8s:deepcopy-gen=false
// InstrumentationValidator is used to validate instrumentations
type InstrumentationValidator struct {
	OperatorNamespace         string
	InstrumentationValidators []InstrumentationSpecValidator
}

// NewInstrumentationValidator is used to crate a new validator
func NewInstrumentationValidator(operatorNamespace string) *InstrumentationValidator {
	v := &InstrumentationValidator{
		OperatorNamespace:         operatorNamespace,
		InstrumentationValidators: defaultInstrumentationSpecValidators,
	}
	return v
}

// ValidateCreate to validate the creation operation
func (r *InstrumentationValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	inst := obj.(*Instrumentation)
	log.FromContext(ctx).V(1).Info("Validating creation of v1beta3.Instrumentation", "name", inst.GetName())
	return r.validate(inst)
}

// ValidateUpdate to validate the update operation
func (r *InstrumentationValidator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	inst := newObj.(*Instrumentation)
	log.FromContext(ctx).V(1).Info("Validating update of v1beta3.Instrumentation", "name", inst.GetName())
	return r.validate(inst)
}

// ValidateDelete to validate the deletion operation
func (r *InstrumentationValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	inst := obj.(*Instrumentation)
	log.FromContext(ctx).V(1).Info("Validating deletion of v1beta3.Instrumentation", "name", inst.GetName())
	return r.validate(inst)
}

// validate to validate all the fields
func (r *InstrumentationValidator) validate(inst *Instrumentation) (admission.Warnings, error) {
	if r.OperatorNamespace != inst.Namespace {
		return nil, fmt.Errorf("instrumentation must be in operator namespace")
	}
	for _, v := range r.InstrumentationValidators {
		if err := v(inst); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

var defaultInstrumentationSpecValidators = []InstrumentationSpecValidator{
	validateLicenseKeySecret,
	validateAgentConfigMap,
	validateContainerSelector,
	validatePodSelector,
	validateNamespaceSelector,
	validateHealthAgent,
	validateAgent,
}

var validImagePullPolicies = map[corev1.PullPolicy]struct{}{
	corev1.PullNever:        {},
	corev1.PullAlways:       {},
	corev1.PullIfNotPresent: {},
	corev1.PullPolicy(""):   {},
}

var acceptableLangs = []string{
	"dotnet",
	"dotnet-windows2022",
	"dotnet-windows2025",
	"go",
	"java",
	"nodejs",
	"php-7.2", "php-7.3", "php-7.4", "php-8.0", "php-8.1", "php-8.2", "php-8.3", "php-8.4", "php-8.5",
	"python",
	"ruby",
}

func validateAgent(inst *Instrumentation) error {
	if inst.Spec.Agent.IsEmpty() {
		return fmt.Errorf("instrumentation %q agent is empty", inst.Name)
	}
	if agentLang := inst.Spec.Agent.Language; !slices.Contains(acceptableLangs, agentLang) {
		return fmt.Errorf("instrumentation agent language %q must be one of the accepted languages (%s)", agentLang, strings.Join(acceptableLangs, ", "))
	}
	if _, ok := validImagePullPolicies[inst.Spec.Agent.ImagePullPolicy]; !ok {
		return fmt.Errorf("instrumentation agent.imagePullPolicy is invalid")
	}
	if err := validateEnvs(inst.Spec.Agent.Env); err != nil {
		return fmt.Errorf("instrumentation agent.env is invalid > %w", err)
	}
	return nil
}

func validateHealthAgent(inst *Instrumentation) error {
	if inst.Spec.HealthAgent.Image == "" {
		err := fmt.Errorf("instrumentation %q healthAgent.image is required if anthing else in the healthAgent is configured", inst.Name)
		if len(inst.Spec.HealthAgent.Env) > 0 {
			return err
		}
		if len(inst.Spec.HealthAgent.ImagePullPolicy) > 0 {
			return err
		}
		if len(inst.Spec.HealthAgent.Resources.Requests) > 0 || len(inst.Spec.HealthAgent.Resources.Limits) > 0 || len(inst.Spec.HealthAgent.Resources.Claims) > 0 {
			return err
		}
		if inst.Spec.HealthAgent.SecurityContext != nil {
			return err
		}
	}
	if _, ok := validImagePullPolicies[inst.Spec.HealthAgent.ImagePullPolicy]; !ok {
		return fmt.Errorf("instrumentation healthAgent.imagePullPolicy is invalid")
	}
	if err := validateEnvs(inst.Spec.HealthAgent.Env); err != nil {
		return fmt.Errorf("instrumentation healthAgent.env is invalid > %w", err)
	}
	return nil
}

func validateNamespaceSelector(inst *Instrumentation) error {
	if _, err := metav1.LabelSelectorAsSelector(&inst.Spec.NamespaceLabelSelector); err != nil {
		return fmt.Errorf("instrumentation namespaceLabelSelector is invalid: %w", err)
	}
	return nil
}

func validatePodSelector(inst *Instrumentation) error {
	if _, err := metav1.LabelSelectorAsSelector(&inst.Spec.PodLabelSelector); err != nil {
		return fmt.Errorf("instrumentation podLabelSelector is invalid: %w", err)
	}
	return nil
}

func validateLicenseKeySecret(inst *Instrumentation) error {
	for _, entry := range inst.Spec.Agent.Env {
		if entry.Name == "NEW_RELIC_LICENSE_KEY" {
			return fmt.Errorf("%q is already set by the licenseKeySecret", entry.Name)
		}
	}
	return nil
}

var acceptLangsForAgentConfigMap = []string{"java"}

func validateAgentConfigMap(inst *Instrumentation) error {
	agentLang := inst.Spec.Agent.Language

	if inst.Spec.AgentConfigMap != "" {
		if !slices.Contains(acceptLangsForAgentConfigMap, inst.Spec.Agent.Language) {
			return fmt.Errorf("instrumentation agent language %q does not support an agentConfigMap, agentConfigMap can only be configured with one of these languages (%q)", agentLang, strings.Join(acceptLangsForAgentConfigMap, ", "))
		}
	}

	if agentLang == "java" && inst.Spec.AgentConfigMap != "" {
		for _, entry := range inst.Spec.Agent.Env {
			if entry.Name == "NEWRELIC_FILE" {
				return fmt.Errorf("%q is already set by the agentConfigMap", entry.Name)
			}
		}
	}
	return nil
}

func validateContainerSelector(inst *Instrumentation) error {
	if _, err := inst.Spec.ContainerSelector.NameSelector.AsSelector(); err != nil {
		return fmt.Errorf("instrumentation containerSelector.nameSelector is invalid: %w", err)
	}
	if _, err := inst.Spec.ContainerSelector.ImageSelector.AsSelector(); err != nil {
		return fmt.Errorf("instrumentation containerSelector.imageSelector is invalid: %w", err)
	}
	if _, err := inst.Spec.ContainerSelector.EnvSelector.AsSelector(); err != nil {
		return fmt.Errorf("instrumentation containerSelector.envSelector is invalid: %w", err)
	}
	return nil
}

// validateEnv to validate the environment variables used all start with the required prefixes
func validateEnvs(env []corev1.EnvVar) error {
	var invalidNames []string
	for _, env := range env {
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
