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

package webhookhandler_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/newrelic/k8s-agents-operator/api/v1alpha1"
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

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.TODO())
	defer cancel()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "config", "webhook")},
		},
	}
	cfg, err = testEnv.Start()
	if err != nil {
		fmt.Printf("failed to start testEnv: %v", err)
		os.Exit(1)
	}

	if err = v1alpha1.AddToScheme(testScheme); err != nil {
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
		Scheme:             testScheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	if mgrErr != nil {
		fmt.Printf("failed to start webhook server: %v", mgrErr)
		os.Exit(1)
	}

	ctx, cancel = context.WithCancel(context.TODO())
	defer cancel()
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

	err = testEnv.Stop()
	if err != nil {
		fmt.Printf("failed to stop testEnv: %v", err)
		os.Exit(1)
	}

	os.Exit(code)
}
