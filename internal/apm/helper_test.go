package apm

import (
	"testing"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/util"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBaseInjector_ConfigureClient(t *testing.T) {

}

func TestBaseInjector_ConfigureLogger(t *testing.T) {

}

func TestInjectorRegistery_Register(t *testing.T) {

}

func TestInjectorRegistery_MustRegister(t *testing.T) {

}

func TestInjectors_Names(t *testing.T) {

}

func TestApplyLabel(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		expectedPod    *corev1.Pod
		expectedErrStr string
	}{
		{
			name: "a container with labels added",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_LABELS", Value: "app:java-injected"},
						}}}}},

			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "NEW_RELIC_LABELS", Value: "app:java-injected"},
						},
					}},
				}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualPod := applyLabelToPod(test.pod, "foo", "bar")

			if diff := cmp.Diff(test.expectedPod, actualPod); diff != "" {
				assert.Fail(t, diff)
			}
		})
	}
}

func TestEncodeDecodeAttributes(t *testing.T) {
	var diff string
	diff = cmp.Diff(map[string]string{"a": "b", "c": "d", "e": "f"}, decodeAttributes("a:b;c:d;e:f", ";", ":"))
	if diff != "" {
		assert.Fail(t, diff)
	}
	diff = cmp.Diff("a:b;c:d;e:f", encodeAttributes(map[string]string{"a": "b", "c": "d", "e": "f"}, ";", ":"))
	if diff != "" {
		assert.Fail(t, diff)
	}
}

func TestGetAppName(t *testing.T) {
	tests := []struct {
		name            string
		containerName   string
		pod             *corev1.Pod
		expectedAppName string
	}{
		{
			name:            "get container name",
			containerName:   "container-name",
			pod:             &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "container-name",
		},
		{
			name:            "get 2nd container name",
			containerName:   "container-name-2",
			pod:             &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}, {Name: "container-name-2"}}}},
			expectedAppName: "container-name-2",
		},
		{
			name:            "get init container name",
			containerName:   "init-container-name",
			pod:             &corev1.Pod{Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "init-container-name"}}, Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "init-container-name",
		},
		{
			name:            "get 2nd init container name",
			containerName:   "init-container-name-2",
			pod:             &corev1.Pod{Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "init-container-name"}, {Name: "init-container-name-2"}}, Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "init-container-name-2",
		},
		{
			name:            "get pod name",
			containerName:   "container-name",
			pod:             &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "pod-name",
		},
		{
			name:            "get container name if pod name generated",
			containerName:   "container-name",
			pod:             &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "container-name",
		},
		{
			name:          "get statefulset name",
			containerName: "container-name",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name", OwnerReferences: []metav1.OwnerReference{
				{Kind: "StatefulSet", Name: "statefulset-name"},
			}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "statefulset-name",
		},
		{
			name:          "get daemonset name",
			containerName: "container-name",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name", OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "daemonset-name"},
			}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "daemonset-name",
		},
		{
			name:          "get deployment name",
			containerName: "container-name",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name", OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "deployment-name"},
			}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "deployment-name",
		},
		{
			name:          "get cronjob name",
			containerName: "container-name",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name", OwnerReferences: []metav1.OwnerReference{
				{Kind: "CronJob", Name: "cronjob-name"},
			}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "cronjob-name",
		},
		{
			name:          "get cronjob name when job is present",
			containerName: "container-name",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name", OwnerReferences: []metav1.OwnerReference{
				{Kind: "Job", Name: "job-name"},
				{Kind: "CronJob", Name: "cronjob-name"},
			}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "cronjob-name",
		},
		{
			name:          "get job name",
			containerName: "container-name",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name", OwnerReferences: []metav1.OwnerReference{
				{Kind: "Job", Name: "job-name"},
			}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "job-name",
		},
		{
			name:          "get replicaset name",
			containerName: "container-name",
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "pod-generated-name", OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "replicaset-name"},
			}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name"}}}},
			expectedAppName: "replicaset-name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			container, _ := util.GetContainerByNameFromPod(test.pod, test.containerName)
			if container == nil {
				t.Errorf("container not found")
				return
			}
			appName := getAppName(test.pod, container)
			if appName != test.expectedAppName {
				t.Errorf("got app name %q, want %q", appName, test.expectedAppName)
			}
		})
	}
}

func TestSetContainerEnvInjectionDefaults(t *testing.T) {
	expectedContainer := corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "NEW_RELIC_APP_NAME", Value: ""},
			{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
			{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
		},
	}
	container := corev1.Container{}
	setContainerEnvInjectionDefaults(&corev1.Pod{}, &container)
	if diff := cmp.Diff(expectedContainer, container); diff != "" {
		assert.Fail(t, diff)
	}
}

func TestSetContainerEnvLicenseKey(t *testing.T) {
	expectedContainer := corev1.Container{
		Env: []corev1.EnvVar{
			{
				Name: EnvNewRelicLicenseKey,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "secret-name",
						},
						Key:      LicenseKey,
						Optional: func() *bool { v := true; return &v }(),
					},
				},
			},
		},
	}
	container := corev1.Container{}
	setContainerEnvLicenseKey(&container, "secret-name")
	if diff := cmp.Diff(expectedContainer, container); diff != "" {
		assert.Fail(t, diff)
	}
}

func TestSetContainerEnvFromInst(t *testing.T) {
	container := corev1.Container{}
	expectedContainer := corev1.Container{Env: []corev1.EnvVar{{Name: "A", Value: "B"}}}
	setContainerEnvFromInst(&container, current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Env: []corev1.EnvVar{{Name: "A", Value: "B"}}}}})
	if diff := cmp.Diff(expectedContainer, container); diff != "" {
		assert.Fail(t, diff)
	}
}
