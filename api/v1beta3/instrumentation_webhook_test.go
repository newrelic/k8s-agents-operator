package v1beta3

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
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
	var actualObj = &Instrumentation{}
	err := (&InstrumentationDefaulter{}).Default(context.Background(), actualObj)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if diff := cmp.Diff(expectedObj, actualObj); diff != "" {
		t.Fatalf("Unexpected diff (-want +got): %v", diff)
	}
}

func TestInstrumentationValidatorNamespaceSelectorGate(t *testing.T) {
	const operatorNs = "newrelic"

	validAgent := Agent{Language: "java", Image: "example.com/agent:latest"}
	namespaceSelector := metav1.LabelSelector{MatchLabels: map[string]string{"team": "checkout"}}
	namespaceSelectorExpr := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "team", Operator: metav1.LabelSelectorOpExists},
		},
	}

	tests := []struct {
		name      string
		namespace string
		selector  metav1.LabelSelector
		wantErr   bool
	}{
		{
			name:      "operator namespace with namespace selector is allowed",
			namespace: operatorNs,
			selector:  namespaceSelector,
			wantErr:   false,
		},
		{
			name:      "operator namespace without namespace selector is allowed",
			namespace: operatorNs,
			wantErr:   false,
		},
		{
			name:      "other namespace without namespace selector is allowed",
			namespace: "checkout",
			wantErr:   false,
		},
		{
			name:      "other namespace with matchLabels namespace selector is rejected",
			namespace: "checkout",
			selector:  namespaceSelector,
			wantErr:   true,
		},
		{
			name:      "other namespace with matchExpressions namespace selector is rejected",
			namespace: "checkout",
			selector:  namespaceSelectorExpr,
			wantErr:   true,
		},
	}

	validator := NewInstrumentationValidator(operatorNs)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instrumentation{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: tt.namespace},
				Spec: InstrumentationSpec{
					Agent:                  validAgent,
					NamespaceLabelSelector: tt.selector,
				},
			}
			_, err := validator.ValidateCreate(context.Background(), inst)
			if tt.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
