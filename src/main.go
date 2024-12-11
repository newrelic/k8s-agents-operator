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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/newrelic/k8s-agents-operator/src/internal/autodetect"
	instrumentationupgrade "github.com/newrelic/k8s-agents-operator/src/internal/migrate/upgrade"
	"github.com/newrelic/k8s-agents-operator/src/internal/webhook"
	"os"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	"strings"
	"time"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
	"github.com/newrelic/k8s-agents-operator/src/internal/config"
	"github.com/newrelic/k8s-agents-operator/src/internal/version"
	routev1 "github.com/openshift/api/route/v1"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	webhookruntime "sigs.k8s.io/controller-runtime/pkg/webhook"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = k8sruntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type tlsConfig struct {
	minVersion   string
	cipherSuites []string
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(v1alpha2.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme)) // TODO: Update this to not use a deprecated method
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the\tflag.BoolVar(&enableHTTP2, \"enable-http2\", false,\n\t\t\"If set, HTTP/2 will be enabled for the metrics and webhook servers\")\n metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhookruntime.NewServer(webhookruntime.Options{
		TLSOpts: tlsOpts,
	})

	operatorNamespace := os.Getenv("OPERATOR_NAMESPACE")
	if operatorNamespace == "" {
		setupLog.Info("env var OPERATOR_NAMESPACE is required")
		os.Exit(1)
	}

	v := version.Get()

	setupLog.Info("Starting the Kubernetes Agents Operator",
		"k8s-agents-operator", v.Operator,
		"running-namespace", operatorNamespace,
		"build-date", v.BuildDate,
		"go-version", v.Go,
		"go-arch", runtime.GOARCH,
		"go-os", runtime.GOOS,
	)

	// TODO: Start determine usage
	restConfig := ctrl.GetConfigOrDie()

	// builds the operator's configuration
	ad, err := autodetect.New(restConfig)
	if err != nil {
		setupLog.Error(err, "failed to setup auto-detect routine")
		os.Exit(1)
	}

	cfg := config.New(
		config.WithLogger(ctrl.Log.WithName("config")),
		config.WithVersion(v),
		config.WithAutoDetect(ad),
	)
	// End determine usage

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		// TODO(user): TLSOpts is used to allow configuring the TLS config used for the server. If certificates are
		// not provided, self-signed certificates will be generated by default. This option is not recommended for
		// production environments as self-signed certificates do not offer the same level of trust and security
		// as certificates issued by a trusted Certificate Authority (CA). The primary risk is potentially allowing
		// unauthorized access to sensitive metrics data. Consider replacing with CertDir, CertName, and KeyName
		// to provide certificates, ensuring the server communicates using trusted and secure certificates.
		TLSOpts: tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// see https://github.com/openshift/library-go/blob/4362aa519714a4b62b00ab8318197ba2bba51cb7/pkg/config/leaderelection/leaderelection.go#L104
	leaseDuration := time.Second * 137
	renewDeadline := time.Second * 107
	retryPeriod := time.Second * 26

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "9f7554c3.newrelic.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
		LeaseDuration: &leaseDuration,
		RenewDeadline: &renewDeadline,
		RetryPeriod:   &retryPeriod,
	}

	watchNamespace, found := os.LookupEnv("WATCH_NAMESPACE")
	if found {
		setupLog.Info("watching namespace(s)", "namespaces", watchNamespace)
	} else {
		setupLog.Info("the env var WATCH_NAMESPACE isn't set, watching all namespaces")
	}

	if watchNamespace != "" {
		nsDefaults := map[string]cache.Config{}
		for _, watchNamespace := range strings.Split(watchNamespace, ",") {
			nsDefaults[watchNamespace] = cache.Config{}
		}
		mgrOptions.Cache = cache.Options{DefaultNamespaces: nsDefaults}
	}

	mgr, err := ctrl.NewManager(restConfig, mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// TODO: Use controller paradigm & investigate below
	ctx := ctrl.SetupSignalHandler()
	err = addDependencies(ctx, mgr, cfg)
	if err != nil {
		setupLog.Error(err, "failed to add/run bootstrap dependencies to the controller manager")
		os.Exit(1)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		// TODO: Inject logger: logger.WithName("instrumentation-validator")
		if err = (&v1alpha2.Instrumentation{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Instrumentation")
			os.Exit(1)
		}

		// Register the Pod mutation webhook
		if err = webhook.SetupWebhookWithManager(mgr, operatorNamespace, ctrl.Log.WithName("mutation-webhook")); err != nil {
			setupLog.Error(err, "unable to register pod mutate webhook")
			os.Exit(1)
		}
	} else {
		ctrl.Log.Info("Webhooks are disabled, operator is running an unsupported mode", "ENABLE_WEBHOOKS", "false")
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func addDependencies(_ context.Context, mgr ctrl.Manager, cfg config.Config) error {
	// run the auto-detect mechanism for the configuration
	err := mgr.Add(manager.RunnableFunc(func(_ context.Context) error {
		return cfg.StartAutoDetect()
	}))
	if err != nil {
		return fmt.Errorf("failed to start the auto-detect mechanism: %w", err)
	}

	// adds the upgrade mechanism to be executed once the manager is ready
	err = mgr.Add(manager.RunnableFunc(func(c context.Context) error {
		u := &instrumentationupgrade.InstrumentationUpgrade{
			Logger: ctrl.Log.WithName("instrumentation-upgrade"),
			Client: mgr.GetClient(),
		}
		return u.ManagedInstances(c)
	}))
	if err != nil {
		return fmt.Errorf("failed to upgrade Instrumentation instances: %w", err)
	}
	return nil
}
