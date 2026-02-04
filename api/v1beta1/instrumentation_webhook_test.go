package v1beta1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateLicenseKeyInEnv(t *testing.T) {
	operatorNamespace := "operator-ns"
	validator := &InstrumentationValidator{
		OperatorNamespace: operatorNamespace,
	}

	tests := []struct {
		name    string
		inst    *Instrumentation
		wantErr bool
		errMsg  string
	}{
		{
			name: "license key in env should fail",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "newrelic/java-agent:latest",
						Env: []corev1.EnvVar{
							{
								Name:  "NEW_RELIC_LICENSE_KEY",
								Value: "secret-key",
							},
						},
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: true,
			errMsg:  "NEW_RELIC_LICENSE_KEY should not be set in agent.env",
		},
		{
			name: "other NEW_RELIC env vars should pass",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "newrelic/java-agent:latest",
						Env: []corev1.EnvVar{
							{
								Name:  "NEW_RELIC_APP_NAME",
								Value: "my-app",
							},
							{
								Name:  "NEW_RELIC_DISTRIBUTED_TRACING_ENABLED",
								Value: "true",
							},
						},
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: false,
		},
		{
			name: "NEWRELIC prefix env vars should pass",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "python",
						Image:    "newrelic/python-agent:latest",
						Env: []corev1.EnvVar{
							{
								Name:  "NEWRELIC_LOG_LEVEL",
								Value: "debug",
							},
						},
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.validate(tt.inst)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidationOrder(t *testing.T) {
	operatorNamespace := "operator-ns"
	validator := &InstrumentationValidator{
		OperatorNamespace: operatorNamespace,
	}

	tests := []struct {
		name    string
		inst    *Instrumentation
		wantErr bool
		errMsg  string
	}{
		{
			name: "empty agent should fail with empty agent error, not language error",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "", // Empty language
						Image:    "", // Empty image
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: true,
			errMsg:  "agent is empty", // Should get this error, not language error
		},
		{
			name: "agent with image but invalid language should fail with language error",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "invalid-lang",
						Image:    "some-image:latest",
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: true,
			errMsg:  "must be one of the accepted languages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.validate(tt.inst)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	if len(s) == 0 || len(substr) == 0 {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestValidateAgentConfigMapWithNewrelicFile(t *testing.T) {
	operatorNamespace := "operator-ns"
	validator := &InstrumentationValidator{
		OperatorNamespace: operatorNamespace,
	}

	tests := []struct {
		name    string
		inst    *Instrumentation
		wantErr bool
		errMsg  string
	}{
		{
			name: "agentConfigMap with NEWRELIC_FILE should fail",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					AgentConfigMap: "my-config-map",
					Agent: Agent{
						Language: "java",
						Image:    "newrelic/java-agent:latest",
						Env: []corev1.EnvVar{
							{
								Name:  "NEWRELIC_FILE",
								Value: "/path/to/config.yml",
							},
						},
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: true,
			errMsg:  "is already set by the agentConfigMap",
		},
		{
			name: "agentConfigMap without NEWRELIC_FILE should pass",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					AgentConfigMap: "my-config-map",
					Agent: Agent{
						Language: "java",
						Image:    "newrelic/java-agent:latest",
						Env: []corev1.EnvVar{
							{
								Name:  "NEW_RELIC_APP_NAME",
								Value: "my-app",
							},
						},
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: false,
		},
		{
			name: "no agentConfigMap with NEWRELIC_FILE should pass",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					Agent: Agent{
						Language: "java",
						Image:    "newrelic/java-agent:latest",
						Env: []corev1.EnvVar{
							{
								Name:  "NEWRELIC_FILE",
								Value: "/path/to/config.yml",
							},
						},
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: false,
		},
		{
			name: "agentConfigMap with empty env should pass",
			inst: &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-inst",
					Namespace: operatorNamespace,
				},
				Spec: InstrumentationSpec{
					AgentConfigMap: "my-config-map",
					Agent: Agent{
						Language: "java",
						Image:    "newrelic/java-agent:latest",
						Env:      []corev1.EnvVar{},
					},
					PodLabelSelector:       metav1.LabelSelector{},
					NamespaceLabelSelector: metav1.LabelSelector{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.validate(tt.inst)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error = %v", err)
				}
			}
		})
	}
}
