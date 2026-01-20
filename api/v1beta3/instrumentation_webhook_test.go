package v1beta3

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestInstrumentationDefaulter(t *testing.T) {
	var expectedObj runtime.Object = &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "k8s-agents-operator",
			},
		},
		Spec: InstrumentationSpec{
			LicenseKeySecret: "newrelic-key-secret",
		},
	}
	var actualObj runtime.Object = &Instrumentation{}
	err := (&InstrumentationDefaulter{}).Default(context.Background(), actualObj)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if diff := cmp.Diff(expectedObj, actualObj); diff != "" {
		t.Fatalf("Unexpected diff (-want +got): %v", diff)
	}
}

func TestValidateAgent_Languages(t *testing.T) {
	tests := []struct {
		name     string
		language string
		wantErr  bool
	}{
		{"java", "java", false},
		{"python", "python", false},
		{"nodejs", "nodejs", false},
		{"ruby", "ruby", false},
		{"go", "go", false},
		{"dotnet", "dotnet", false},
		{"dotnet-windows2022", "dotnet-windows2022", false},
		{"php-8.3", "php-8.3", false},
		{"invalid", "invalid-lang", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instrumentation{
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: tt.language,
						Image:    "agent:latest",
					},
				},
			}

			err := validateAgent(inst)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAgent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAgent_ImagePullPolicy(t *testing.T) {
	tests := []struct {
		name            string
		imagePullPolicy corev1.PullPolicy
		wantErr         bool
	}{
		{"Always", corev1.PullAlways, false},
		{"Never", corev1.PullNever, false},
		{"IfNotPresent", corev1.PullIfNotPresent, false},
		{"Empty", "", false},
		{"Invalid", "InvalidPolicy", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instrumentation{
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language:        "java",
						Image:           "agent:latest",
						ImagePullPolicy: tt.imagePullPolicy,
					},
				},
			}

			err := validateAgent(inst)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAgent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateHealthAgent_ResourcesWithoutImage(t *testing.T) {
	inst := &Instrumentation{
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
			},
			HealthAgent: HealthAgent{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				},
			},
		},
	}

	err := validateHealthAgent(inst)
	if err == nil {
		t.Error("validateHealthAgent() expected error for resources without image, got nil")
	}
}

func TestValidateLicenseKeySecret_InEnv(t *testing.T) {
	inst := &Instrumentation{
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
				Env: []corev1.EnvVar{
					{Name: "NEW_RELIC_LICENSE_KEY", Value: "secret"},
				},
			},
		},
	}

	err := validateLicenseKeySecret(inst)
	if err == nil {
		t.Error("validateLicenseKeySecret() expected error for license key in env, got nil")
	}
}

func TestValidateAgentConfigMap_UnsupportedLanguage(t *testing.T) {
	inst := &Instrumentation{
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "python",
				Image:    "agent:latest",
			},
			AgentConfigMap: "python-config",
		},
	}

	err := validateAgentConfigMap(inst)
	if err == nil {
		t.Error("validateAgentConfigMap() expected error for python with configmap, got nil")
	}
}

func TestValidateEnvs_ValidPrefixes(t *testing.T) {
	tests := []struct {
		name    string
		envVars []corev1.EnvVar
		wantErr bool
	}{
		{
			name: "valid NEW_RELIC_ prefix",
			envVars: []corev1.EnvVar{
				{Name: "NEW_RELIC_APP_NAME", Value: "test"},
			},
			wantErr: false,
		},
		{
			name: "valid NEWRELIC_ prefix",
			envVars: []corev1.EnvVar{
				{Name: "NEWRELIC_APP_NAME", Value: "test"},
			},
			wantErr: false,
		},
		{
			name: "invalid prefix",
			envVars: []corev1.EnvVar{
				{Name: "INVALID_VAR", Value: "test"},
			},
			wantErr: true,
		},
		{
			name: "mixed valid and invalid",
			envVars: []corev1.EnvVar{
				{Name: "NEW_RELIC_APP_NAME", Value: "test"},
				{Name: "INVALID_VAR", Value: "test"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnvs(tt.envVars)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEnvs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePodSelector_Invalid(t *testing.T) {
	inst := &Instrumentation{
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
			},
			PodLabelSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "InvalidOperator",
					},
				},
			},
		},
	}

	err := validatePodSelector(inst)
	if err == nil {
		t.Error("validatePodSelector() expected error for invalid selector, got nil")
	}
}

func TestInstrumentationValidator_NamespaceValidation(t *testing.T) {
	validator := NewInstrumentationValidator("newrelic")

	tests := []struct {
		name          string
		instNamespace string
		wantErr       bool
	}{
		{"same namespace", "newrelic", false},
		{"different namespace", "default", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: tt.instNamespace,
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
				},
			}

			_, err := validator.ValidateCreate(context.Background(), inst)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateContainerSelector(t *testing.T) {
	tests := []struct {
		name            string
		inst            *Instrumentation
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "valid name selector",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inst",
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
					ContainerSelector: ContainerSelector{
						NameSelector: NameSelector{
							MatchNames: map[string]string{
								"container": "app",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid name selector expression",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inst",
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
					ContainerSelector: ContainerSelector{
						NameSelector: NameSelector{
							MatchExpressions: []NameSelectorRequirement{
								{
									Key:      "InvalidKey",
									Operator: "InvalidOperator",
									Values:   []string{"value"},
								},
							},
						},
					},
				},
			},
			wantErr:         true,
			wantErrContains: "nameSelector is invalid",
		},
		{
			name: "valid image selector",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inst",
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
					ContainerSelector: ContainerSelector{
						ImageSelector: ImageSelector{
							MatchImages: map[string]string{
								"url": "myapp:*",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid image selector expression",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inst",
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
					ContainerSelector: ContainerSelector{
						ImageSelector: ImageSelector{
							MatchExpressions: []ImageSelectorRequirement{
								{
									Key:      "InvalidKey",
									Operator: "InvalidOperator",
									Values:   []string{"value"},
								},
							},
						},
					},
				},
			},
			wantErr:         true,
			wantErrContains: "imageSelector is invalid",
		},
		{
			name: "valid env selector",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inst",
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
					ContainerSelector: ContainerSelector{
						EnvSelector: EnvSelector{
							MatchEnvs: map[string]string{
								"APP_ENV": "prod",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid env selector expression",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inst",
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
					ContainerSelector: ContainerSelector{
						EnvSelector: EnvSelector{
							MatchExpressions: []EnvSelectorRequirement{
								{
									Key:      "APP_ENV",
									Operator: "InvalidOperator",
									Values:   []string{"prod"},
								},
							},
						},
					},
				},
			},
			wantErr:         true,
			wantErrContains: "envSelector is invalid",
		},
		{
			name: "multiple valid selectors",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-inst",
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "agent:latest",
					},
					ContainerSelector: ContainerSelector{
						NameSelector: NameSelector{
							MatchNames: map[string]string{
								"container": "app",
							},
						},
						ImageSelector: ImageSelector{
							MatchImages: map[string]string{
								"url": "myapp:*",
							},
						},
						EnvSelector: EnvSelector{
							MatchEnvs: map[string]string{
								"APP_ENV": "prod",
							},
						},
						NamesFromPodAnnotation: "container.names",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerSelector(tt.inst)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateContainerSelector() expected error, got nil")
				} else if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("validateContainerSelector() error = %v, want error containing %q", err, tt.wantErrContains)
				}
			} else {
				if err != nil {
					t.Errorf("validateContainerSelector() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateAgent_EmptyImage(t *testing.T) {
	inst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inst",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "", // Empty image
			},
		},
	}

	err := validateAgent(inst)
	if err == nil {
		t.Error("validateAgent() expected error for empty image, got nil")
	}
}

func TestValidateAgent_SecurityContext(t *testing.T) {
	runAsUser := int64(1000)
	inst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inst",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
				SecurityContext: &corev1.SecurityContext{
					RunAsUser: &runAsUser,
				},
			},
		},
	}

	err := validateAgent(inst)
	if err != nil {
		t.Errorf("validateAgent() unexpected error for agent with security context = %v", err)
	}
}

func TestValidateHealthAgent_SecurityContext(t *testing.T) {
	runAsNonRoot := true
	inst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inst",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
			},
			HealthAgent: HealthAgent{
				Image: "health-agent:latest",
				SecurityContext: &corev1.SecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
			},
		},
	}

	err := validateHealthAgent(inst)
	if err != nil {
		t.Errorf("validateHealthAgent() unexpected error for health agent with security context = %v", err)
	}
}

func TestValidateNamespaceSelector_InvalidExpression(t *testing.T) {
	inst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inst",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
			},
			NamespaceLabelSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "env",
						Operator: "InvalidOperator",
						Values:   []string{"prod"},
					},
				},
			},
		},
	}

	err := validateNamespaceSelector(inst)
	if err == nil {
		t.Error("validateNamespaceSelector() expected error for invalid operator, got nil")
	}
}

func TestValidateAgent_VolumeSizeLimit(t *testing.T) {
	volumeSize := resource.MustParse("2Gi")
	inst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inst",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language:        "java",
				Image:           "agent:latest",
				VolumeSizeLimit: &volumeSize,
			},
		},
	}

	err := validateAgent(inst)
	if err != nil {
		t.Errorf("validateAgent() unexpected error for agent with volume size limit = %v", err)
	}
}

func TestValidateHealthAgent_ImagePullPolicyEmpty(t *testing.T) {
	inst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-inst",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
			},
			HealthAgent: HealthAgent{
				Image:           "health-agent:latest",
				ImagePullPolicy: "", // Empty is valid, uses default
			},
		},
	}

	err := validateHealthAgent(inst)
	if err != nil {
		t.Errorf("validateHealthAgent() unexpected error for empty image pull policy = %v", err)
	}
}

func TestInstrumentationValidator_ValidateUpdate(t *testing.T) {
	validator := NewInstrumentationValidator("newrelic")

	oldInst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-inst",
			Namespace: "newrelic",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:v1",
			},
		},
	}

	newInst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-inst",
			Namespace: "newrelic",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:v2",
			},
		},
	}

	_, err := validator.ValidateUpdate(context.Background(), oldInst, newInst)
	if err != nil {
		t.Errorf("ValidateUpdate() unexpected error = %v", err)
	}
}

func TestInstrumentationValidator_ValidateDelete(t *testing.T) {
	validator := NewInstrumentationValidator("newrelic")

	inst := &Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-inst",
			Namespace: "newrelic",
		},
		Spec: InstrumentationSpec{
			Agent: Agent{
				Language: "java",
				Image:    "agent:latest",
			},
		},
	}

	_, err := validator.ValidateDelete(context.Background(), inst)
	if err != nil {
		t.Errorf("ValidateDelete() unexpected error = %v", err)
	}
}
