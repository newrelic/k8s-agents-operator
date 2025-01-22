package v1alpha2

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/newrelic/k8s-agents-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestConvertTo(t *testing.T) {
	tests := []struct {
		name           string
		expectedErrStr string
		input          Instrumentation
		expectedOutput v1beta1.Instrumentation
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
			expectedOutput: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
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
					Agent: v1beta1.Agent{
						Image:    "f",
						Language: "g",
						Env: []corev1.EnvVar{
							{Name: "h", Value: "i"},
						},
					},
					HealthAgent: v1beta1.HealthAgent{
						Image: "",
					},
				},
			},
		},
	}
	for ti, test := range tests {
		t.Run(fmt.Sprintf("%d_%s", ti, test.name), func(t *testing.T) {
			errStr := ""
			out := v1beta1.Instrumentation{}
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
		input          v1beta1.Instrumentation
		expectedOutput Instrumentation
	}{
		{
			name:           "everything matches",
			expectedErrStr: "",
			input: v1beta1.Instrumentation{
				Spec: v1beta1.InstrumentationSpec{
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
					Agent: v1beta1.Agent{
						Image:    "f",
						Language: "g",
						Env: []corev1.EnvVar{
							{Name: "h", Value: "i"},
						},
					},
					HealthAgent: v1beta1.HealthAgent{
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
