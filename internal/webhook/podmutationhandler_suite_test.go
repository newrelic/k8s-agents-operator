/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	webhookruntime "sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/api/v1alpha2"
	"github.com/newrelic/k8s-agents-operator/api/v1beta1"
	"github.com/newrelic/k8s-agents-operator/internal/apm"
	"github.com/newrelic/k8s-agents-operator/internal/instrumentation"
	"github.com/newrelic/k8s-agents-operator/internal/version"
	"github.com/newrelic/k8s-agents-operator/internal/webhook"
	// +kubebuilder:scaffold:imports
)

var (
	k8sClient  client.Client
	testEnv    *envtest.Environment
	testScheme *runtime.Scheme = scheme.Scheme
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	cfg        *rest.Config
)

var _ io.Writer = (*fakeWriter)(nil)

type fakeWriter struct{}

func (w *fakeWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.TODO())
	defer cancel()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},

		ErrorIfCRDPathMissing: false,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.29.0-%s-%s", stdruntime.GOOS, stdruntime.GOARCH)),

		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "config", "webhook")},
		},
	}
	cfg, err = testEnv.Start()
	if err != nil {
		fmt.Printf("failed to start testEnv: %v", err)
		os.Exit(1)
	}

	if err = v1alpha2.AddToScheme(testScheme); err != nil {
		fmt.Printf("failed to register scheme: %v", err)
		os.Exit(1)
	}

	if err = v1beta1.AddToScheme(testScheme); err != nil {
		fmt.Printf("failed to register scheme: %v", err)
		os.Exit(1)
	}

	if err = current.AddToScheme(testScheme); err != nil {
		fmt.Printf("failed to register scheme: %v", err)
		os.Exit(1)
	}

	if err = admissionv1.AddToScheme(testScheme); err != nil {
		fmt.Printf("failed to register scheme: %v", err)
		os.Exit(1)
	}

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Printf("failed to setup a Kubernetes client: %v", err)
		os.Exit(1)
	}

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, mgrErr := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
		WebhookServer: webhookruntime.NewServer(webhookruntime.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
	})
	if mgrErr != nil {
		fmt.Printf("failed to start webhook server: %v", mgrErr)
		os.Exit(1)
	}

	operatorNamespace := "newrelic"
	injectorRegistry := apm.DefaultInjectorRegistry

	v1alpha2InstDefaulter := &v1alpha2.InstrumentationDefaulter{}
	v1alpha2InstValidator := &v1alpha2.InstrumentationValidator{
		OperatorNamespace: operatorNamespace,
	}
	err = ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha2.Instrumentation{}).
		WithValidator(v1alpha2InstValidator).
		WithDefaulter(v1alpha2InstDefaulter).
		Complete()
	if err != nil {
		fmt.Printf("failed to register v1alpha2.instrumentation webhook: %v", err)
		os.Exit(1)
	}

	v1beta1InstDefaulter := &v1beta1.InstrumentationDefaulter{}
	v1beta1InstValidator := &v1beta1.InstrumentationValidator{
		OperatorNamespace: operatorNamespace,
	}
	err = ctrl.NewWebhookManagedBy(mgr).
		For(&v1beta1.Instrumentation{}).
		WithValidator(v1beta1InstValidator).
		WithDefaulter(v1beta1InstDefaulter).
		Complete()
	if err != nil {
		fmt.Printf("failed to register v1beta1.instrumentation webhook: %v", err)
		os.Exit(1)
	}

	currentInstDefaulter := &current.InstrumentationDefaulter{}
	currentInstValidator := &current.InstrumentationValidator{
		OperatorNamespace: operatorNamespace,
	}
	err = ctrl.NewWebhookManagedBy(mgr).
		For(&current.Instrumentation{}).
		WithValidator(currentInstValidator).
		WithDefaulter(currentInstDefaulter).
		Complete()
	if err != nil {
		fmt.Printf("failed to register current.instrumentation webhook: %v", err)
		os.Exit(1)
	}

	client := mgr.GetClient()
	injector := instrumentation.NewNewrelicSdkInjector(client, injectorRegistry)
	secretReplicator := instrumentation.NewNewrelicSecretReplicator(client)
	configMapReplicator := instrumentation.NewNewrelicConfigMapReplicator(client)
	instrumentationLocator := instrumentation.NewNewRelicInstrumentationLocator(client, operatorNamespace)
	mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhookruntime.Admission{
		Handler: &webhook.PodMutationHandler{
			Client:  client,
			Decoder: admission.NewDecoder(mgr.GetScheme()),
			Mutators: []webhook.PodMutator{
				instrumentation.NewMutator(
					client,
					injector,
					secretReplicator,
					configMapReplicator,
					instrumentationLocator,
					operatorNamespace,
				),
			},
		}})

	go func() {
		if err = mgr.Start(ctx); err != nil {
			fmt.Printf("failed to start manager: %v", err)
			os.Exit(1)
		}
	}()

	// wait for the webhook server to get ready
	wg := &sync.WaitGroup{}
	wg.Add(1)
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		if err = retry.OnError(wait.Backoff{
			Steps:    20,
			Duration: 10 * time.Millisecond,
			Factor:   1.5,
			Jitter:   0.1,
			Cap:      time.Second * 30,
		}, func(error) bool {
			return true
		}, func() error {
			// #nosec G402
			conn, tlsErr := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
			if tlsErr != nil {
				return tlsErr
			}
			_ = conn.Close()
			return nil
		}); err != nil {
			fmt.Printf("failed to wait for webhook server to be ready: %v", err)
			os.Exit(1)
		}
	}(wg)
	wg.Wait()

	code := m.Run()

	cancel()
	err = testEnv.Stop()
	if err != nil {
		fmt.Printf("failed to stop testEnv: %v", err)
		os.Exit(1)
	}

	os.Exit(code)
}

func TestPodMutationHandler_Handle(t *testing.T) {
	optionalTrue := true

	tests := []struct {
		name                 string
		initNamespaces       []corev1.Namespace
		initSecrets          []corev1.Secret
		initInstrumentations []current.Instrumentation
		initPod              corev1.Pod
		expectedPod          corev1.Pod
	}{
		{
			name: "basic",
			initNamespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			initSecrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{Name: instrumentation.DefaultLicenseKeySecretName, Namespace: "newrelic"},
					Data:       map[string][]byte{apm.LicenseKey: []byte("fake-secret-abc123")},
				},
			},
			initInstrumentations: []current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation-python", Namespace: "newrelic"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"inject": "python"},
						},
						Agent: current.Agent{
							Language: "python",
							Image:    "not-a-real-python-image",
						},
					},
				},
			},
			initPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "alpine1", Namespace: "default", Labels: map[string]string{"inject": "python"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "alpine",
							Image:   "alpine:latest",
							Command: []string{"sleep", "300"},
						},
					},
				},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine1",
					Namespace: "default",
					Labels: map[string]string{
						"inject":                                 "python",
						apm.DescK8sAgentOperatorVersionLabelName: version.Get().Operator,
					},
					Generation: 1,
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:    "nri-python--alpine",
							Image:   "not-a-real-python-image",
							Command: []string{"cp", "-a", "/instrumentation/.", "/nri-python--alpine/"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name: "nri-python--alpine", MountPath: "/nri-python--alpine",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "alpine",
							Image:   "alpine:latest",
							Command: []string{"sleep", "300"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name: "nri-python--alpine", MountPath: "/nri-python--alpine",
								},
							},
							Env: []corev1.EnvVar{
								{Name: "PYTHONPATH", Value: "/nri-python--alpine"},
								{Name: "NEW_RELIC_APP_NAME", Value: "alpine1"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "new_relic_license_key", Optional: &optionalTrue, LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}}}},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "nri-python--alpine",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		{
			name: "test php",
			initNamespaces: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic"}},
			},
			initSecrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{Name: instrumentation.DefaultLicenseKeySecretName, Namespace: "newrelic"},
					Data:       map[string][]byte{apm.LicenseKey: []byte("fake-secret-abc123")},
				},
			},
			initInstrumentations: []current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentation-php", Namespace: "newrelic"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"inject": "php",
							},
						},
						Agent: current.Agent{
							Language: "php-8.3",
							Image:    "not-a-real-php-image",
						},
					},
				},
			},
			initPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "alpine2", Namespace: "default", Labels: map[string]string{"inject": "php"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "alpine",
							Image:   "alpine:latest",
							Command: []string{"sleep", "300"},
							Env: []corev1.EnvVar{
								{Name: "a", Value: "a"},
								{Name: "b", Value: "b"},
							},
						},
					},
				},
			},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "alpine2", Namespace: "default", Labels: map[string]string{
					"inject":                                 "php",
					apm.DescK8sAgentOperatorVersionLabelName: version.Get().Operator},
					Generation: 1,
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:    "nri-php--alpine",
							Image:   "not-a-real-php-image",
							Command: []string{"/bin/sh"},
							Args: []string{"-c", strings.Join([]string{
								"cp -a /instrumentation/. /nri-php--alpine/",
								"sed -i 's@/newrelic-instrumentation@/nri-php--alpine@g' /nri-php--alpine/php-agent/ini/newrelic.ini",
								"sed -i 's@/newrelic-instrumentation@/nri-php--alpine@g' /nri-php--alpine/k8s-php-install.sh",
								"sed -i 's@/newrelic-instrumentation@/nri-php--alpine@g' /nri-php--alpine/nr_env_to_ini.sh",
								"/nri-php--alpine/k8s-php-install.sh 20230831",
								"/nri-php--alpine/nr_env_to_ini.sh",
							}, " && ")},
							Env: []corev1.EnvVar{
								{Name: "NEW_RELIC_APP_NAME", Value: "alpine2"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "new_relic_license_key", Optional: &optionalTrue, LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}}}},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name: "nri-php--alpine", MountPath: "/nri-php--alpine",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "alpine",
							Image:   "alpine:latest",
							Command: []string{"sleep", "300"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name: "nri-php--alpine", MountPath: "/nri-php--alpine",
								},
							},
							Env: []corev1.EnvVar{
								{Name: "a", Value: "a"},
								{Name: "b", Value: "b"},
								{Name: "PHP_INI_SCAN_DIR", Value: ":/nri-php--alpine/php-agent/ini"},
								{Name: "NEW_RELIC_APP_NAME", Value: "alpine2"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "nri-php--alpine",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	opts := cmp.Options{
		cmpopts.IgnoreFields(corev1.PodSpec{},
			"DNSPolicy",
			"EnableServiceLinks",
			"PreemptionPolicy",
			"Priority",
			"RestartPolicy",
			"SchedulerName",
			"SecurityContext",
			"TerminationGracePeriodSeconds",
			"Tolerations",
		),
		cmpopts.IgnoreFields(metav1.ObjectMeta{},
			"CreationTimestamp",
			"ManagedFields",
			"ResourceVersion",
			"Annotations",
			"UID",
		),
		cmpopts.IgnoreFields(corev1.Container{},
			"ImagePullPolicy",
			"TerminationMessagePath",
			"TerminationMessagePolicy",
		),
		cmpopts.IgnoreTypes(corev1.PodStatus{}),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, ns := range tt.initNamespaces {
				{
					getNs := &corev1.Namespace{}
					err = k8sClient.Get(ctx, client.ObjectKey{Name: ns.Name}, getNs)
					if err == nil {
						continue
					}
					if !apierrors.IsNotFound(err) {
						if err != nil {
							t.Fatalf("failed to check for existing namespace: %v", err)
						}
					}
				}
				err = k8sClient.Create(context.Background(), &ns)
				if err != nil {
					t.Fatalf("failed to create namespace: %v", err)
				}
			}
			for _, secret := range tt.initSecrets {
				{
					getNs := &corev1.Secret{}
					err = k8sClient.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: secret.Namespace}, getNs)
					if err == nil {
						continue
					}
					if !apierrors.IsNotFound(err) {
						if err != nil {
							t.Fatalf("failed to check for existing secret: %v", err)
						}
					}
				}
				err = k8sClient.Create(context.Background(), &secret)
				if err != nil {
					t.Fatalf("failed to create secret: %v", err)
				}
			}
			for _, instrumentation := range tt.initInstrumentations {
				err = k8sClient.Create(context.Background(), &instrumentation)
				if err != nil {
					t.Fatalf("failed to create instrumentation: %v", err)
				}
			}

			err = k8sClient.Create(context.Background(), &tt.initPod)
			if err != nil {
				t.Fatalf("failed to create pod: %v", err)
			}
			if len(tt.initPod.Spec.InitContainers) != 1 {
				t.Errorf("expected 1 init container; got %d", len(tt.initPod.Spec.InitContainers))
			}
			diff := cmp.Diff(tt.expectedPod, tt.initPod, opts...)
			if diff != "" {
				t.Errorf("init pod does not match expected pod (-want +got): %s", diff)
			}
		})
	}
}
