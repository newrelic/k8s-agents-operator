package apm

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
)

func TestGoInjector_Language(t *testing.T) {
	require.Equal(t, "go", (&GoInjector{}).Language())
}

func TestGoInjector_Inject(t *testing.T) {
	vtrue := true
	var vzero int64
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		inst           v1alpha2.Instrumentation
		expectedPod    corev1.Pod
		expectedErrStr string
	}{
		{
			name: "nothing",
		},
		{
			name: "a container, no instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
		},
		{
			name: "a container, wrong instrumentation (not the correct lang)",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "not-this"}}},
		},
		{
			name: "a container, instrumentation with blank licenseKeySecret",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedErrStr: "licenseKeySecret must not be blank",
			inst:           v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "go"}}},
		},
		{
			name: "a container, instrumentation",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test"}}}},
			expectedPod: corev1.Pod{
				Spec: corev1.PodSpec{
					ShareProcessNamespace: &vtrue,
					Containers: []corev1.Container{
						{Name: "test"},
						{
							Name:  "opentelemetry-auto-instrumentation",
							Image: "",
							Env: []corev1.EnvVar{
								{Name: "OTEL_SERVICE_NAME", Value: "test"},
								{Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
								{Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
								{Name: "OTEL_RESOURCE_ATTRIBUTES", Value: "k8s.container.name=test,k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.name=$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)"},
							},
							VolumeMounts:    []corev1.VolumeMount{{Name: "kernel-debug", MountPath: "/sys/kernel/debug"}},
							SecurityContext: &corev1.SecurityContext{Privileged: &vtrue, RunAsUser: &vzero},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "kernel-debug", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/sys/kernel/debug"}}},
					},
				},
			},
			inst: v1alpha2.Instrumentation{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "go"}, LicenseKeySecret: "newrelic-key-secret"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &GoInjector{}
			actualPod, err := i.Inject(ctx, test.inst, test.ns, test.pod)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			require.Equal(t, test.expectedErrStr, errStr)
			if diff := cmp.Diff(test.expectedPod, actualPod); diff != "" {
				assert.Fail(t, diff)
			}
		})
	}
}
