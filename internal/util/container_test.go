package util

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

func TestEnvToMap(t *testing.T) {
	env := []corev1.EnvVar{
		{Name: "a", Value: "d"},
		{Name: "b", Value: "e"},
		{Name: "c", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "c"}}},
	}
	expectedMap := map[string]string{
		"a": "d",
		"b": "e",
	}
	actualMap := EnvToMap(env)
	if diff := cmp.Diff(expectedMap, actualMap); diff != "" {
		t.Errorf("EnvToMap() mismatch (-want +got):\n%s", diff)
	}
}

func TestParseContainerImage(t *testing.T) {
	expectedMap := map[string]string{"url": "a://b:45/c/d:e@f:g"}
	actualMap := ParseContainerImage("a://b:45/c/d:e@f:g")
	if diff := cmp.Diff(expectedMap, actualMap); diff != "" {
		t.Errorf("ParseContainerImage() mismatch (-want +got):\n%s", diff)
	}
}
