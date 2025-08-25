package apm

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
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
		podName         string
		expectedAppName string
		expectedErrStr  string
	}{
		{
			name:            "get deployment name",
			podName:         "pod-name-1",
			expectedAppName: "deployment-name",
		},
		{
			name:            "get cronjob name",
			podName:         "pod-name-2",
			expectedAppName: "cronjob-name",
		},
		{
			name:            "get deployment name",
			podName:         "pod-name-3",
			expectedAppName: "statefulset-name",
		},
		{
			name:            "get deployment name",
			podName:         "pod-name-4",
			expectedAppName: "daemonset-name",
		},
		{
			name:            "get replicaset name (no parent deployment)",
			podName:         "pod-name-5",
			expectedAppName: "replicaset-name-2",
		},
		{
			name:            "get job name (no parent cronjob)",
			podName:         "pod-name-6",
			expectedAppName: "job-name-2",
		},
		{
			name:            "get pod name (no parent replicaset)",
			podName:         "pod-name-7",
			expectedAppName: "pod-name-7",
		},
	}

	ctx := context.Background()
	objs := []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name-1", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "replicaset-name-1", APIVersion: "appsv1", UID: "1"}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}}},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "replicaset-name-1", UID: "1", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "deployment-name", APIVersion: "appsv1", UID: "2"}}},
			Spec: appsv1.ReplicaSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "pod"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "deployment-name", UID: "2"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "pod"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
				},
			},
		},

		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name-2", OwnerReferences: []metav1.OwnerReference{{Kind: "Job", Name: "job-name-1", APIVersion: "batchv1", UID: "3"}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}}},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "job-name-1", UID: "3", OwnerReferences: []metav1.OwnerReference{{Kind: "CronJob", Name: "cronjob-name", APIVersion: "batchv1", UID: "4"}}},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "job"}},
					Spec:       corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
				},
			},
		},
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: "cronjob-name", UID: "4"},
			Spec: batchv1.CronJobSpec{
				Schedule: "* * * * *",
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "job"}},
							Spec:       corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
						},
					},
				},
			},
		},

		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name-3", OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "statefulset-name", APIVersion: "appsv1", UID: "5"}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}}},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "statefulset-name", UID: "5"},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "statefulset"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "statefulset"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
				},
			},
		},

		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name-4", OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "daemonset-name", APIVersion: "appsv1", UID: "6"}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}}},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "daemonset-name", UID: "6"},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "daemonset"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "daemonset"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
				},
			},
		},

		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name-5", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "replicaset-name-2", APIVersion: "appsv1", UID: "7"}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}}},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "replicaset-name-2", UID: "7"},
			Spec: appsv1.ReplicaSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "pod"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
				},
			},
		},

		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name-6", OwnerReferences: []metav1.OwnerReference{{Kind: "Job", Name: "job-name-2", APIVersion: "batchv1", UID: "8"}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}}},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "job-name-2", UID: "8"},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "job"}},
					Spec:       corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, Containers: []corev1.Container{{Name: "container-name", Image: "none"}}},
				},
			},
		},

		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-name-7"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "container-name", Image: "none"}}}},
	}

	for _, obj := range objs {
		obj.SetNamespace("default")
		err := k8sClient.Create(ctx, obj)
		if err != nil {
			t.Logf("failed to create object: %s, %v", err.Error(), obj)
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
			pod := corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: test.podName}, &pod)
			if err != nil {
				t.Fatalf("failed to fetch test pod, %s", err.Error())
			}
			appName, err := (&baseInjector{client: k8sClient}).getRootResourceName(ctx, ns, &pod)
			var errStr string
			if err != nil {
				errStr = err.Error()
			}
			if appName != test.expectedAppName {
				t.Errorf("got app name %q, want %q", appName, test.expectedAppName)
			}
			if errStr != test.expectedErrStr {
				t.Errorf("got error %q, want error %q", errStr, test.expectedErrStr)
			}
		})
	}
}

func TestSetContainerEnvInjectionDefaults(t *testing.T) {
	expectedContainer := corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
			{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
		},
	}
	container := corev1.Container{}
	setContainerEnvInjectionDefaults(&container)
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

func TestGenerateContainerName(t *testing.T) {
	assert.Equal(t, "test"+strings.Repeat("-", 59), generateContainerName("test"+strings.Repeat("-", 59)))
	assert.Equal(t, "test-272f74c", generateContainerName("test"+strings.Repeat("-", 60)))
	assert.Equal(t, "test"+strings.Repeat("x", 51)+"-58def81", generateContainerName("test"+strings.Repeat("x", 60)))
}
