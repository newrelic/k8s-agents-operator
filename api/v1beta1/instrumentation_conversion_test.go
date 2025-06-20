package v1beta1

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/newrelic/k8s-agents-operator/api/current"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestConvertTo(t *testing.T) {
	tests := []struct {
		name           string
		expectedErrStr string
		input          Instrumentation
		expectedOutput current.Instrumentation
	}{
		{
			name:           "first",
			expectedErrStr: "",
			input: Instrumentation{
				Spec: InstrumentationSpec{
					PodLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "c",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"d"},
							},
						},
					},
					NamespaceLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"b": "c"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "d",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"e"},
							},
						},
					},
					Agent: Agent{
						Image:    "f",
						Language: "g",
						Env: []corev1.EnvVar{
							{Name: "h", Value: "i"},
						},
					},
				},
			},
			expectedOutput: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					PodLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "c",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"d"},
							},
						},
					},
					NamespaceLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"b": "c"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "d",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"e"},
							},
						},
					},
					Agent: current.Agent{
						Image:    "f",
						Language: "g",
						Env: []corev1.EnvVar{
							{Name: "h", Value: "i"},
						},
					},
					HealthAgent: current.HealthAgent{},
				},
			},
		},
	}
	for ti, test := range tests {
		t.Run(fmt.Sprintf("%d_%s", ti, test.name), func(t *testing.T) {
			errStr := ""
			out := current.Instrumentation{}
			err := test.input.ConvertTo(&out)
			if err != nil {
				errStr = err.Error()
			}
			if errStr != test.expectedErrStr {
				t.Error(fmt.Errorf("expected error %q, got %q", test.expectedErrStr, errStr))
			}
			if diff := cmp.Diff(test.expectedOutput, out); diff != "" {
				t.Error(fmt.Errorf("unexpected changes: %s", diff))
			}
		})
	}
}

func TestConvertFrom(t *testing.T) {

	tests := []struct {
		name           string
		expectedErrStr string
		input          current.Instrumentation
		expectedOutput Instrumentation
	}{
		{
			name:           "everything matches",
			expectedErrStr: "",
			input: current.Instrumentation{
				Spec: current.InstrumentationSpec{
					PodLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "c",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"d"},
							},
						},
					},
					NamespaceLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"b": "c"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "d",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"e"},
							},
						},
					},
					Agent: current.Agent{
						Image:    "f",
						Language: "g",
						Env: []corev1.EnvVar{
							{Name: "h", Value: "i"},
						},
					},
					AgentConfigMap:   "m",
					LicenseKeySecret: "n",
					HealthAgent: current.HealthAgent{
						Image: "j",
						Env: []corev1.EnvVar{
							{
								Name: "k", Value: "l",
							},
						},
					},
				},
			},
			expectedOutput: Instrumentation{
				Spec: InstrumentationSpec{
					PodLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "c",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"d"},
							},
						},
					},
					NamespaceLabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"b": "c"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "d",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"e"},
							},
						},
					},
					Agent: Agent{
						Image:    "f",
						Language: "g",
						Env: []corev1.EnvVar{
							{Name: "h", Value: "i"},
						},
					},
					AgentConfigMap:   "m",
					LicenseKeySecret: "n",
					HealthAgent: HealthAgent{
						Image: "j",
						Env: []corev1.EnvVar{
							{Name: "k", Value: "l"},
						},
					},
				},
			},
		},
	}
	for ti, test := range tests {
		t.Run(fmt.Sprintf("%d_%s", ti, test.name), func(t *testing.T) {
			errStr := ""
			out := Instrumentation{}
			err := out.ConvertFrom(&test.input)
			if err != nil {
				errStr = err.Error()
			}
			if errStr != test.expectedErrStr {
				t.Error(fmt.Errorf("expected error %q, got %q", test.expectedErrStr, errStr))
			}
			if diff := cmp.Diff(test.expectedOutput, out); diff != "" {
				t.Error(fmt.Errorf("unexpected changes: %s", diff))
			}
		})
	}
}
