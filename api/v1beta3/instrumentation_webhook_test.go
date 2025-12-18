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
	var actualObj runtime.Object = &Instrumentation{}
	err := (&InstrumentationDefaulter{}).Default(context.Background(), actualObj)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if diff := cmp.Diff(expectedObj, actualObj); diff != "" {
		t.Fatalf("Unexpected diff (-want +got): %v", diff)
	}
}
