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
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	openshift_routev1 "github.com/openshift/api/route/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	webhookruntime "sigs.k8s.io/controller-runtime/pkg/webhook"

	newreliccomv1alpha2 "github.com/newrelic/k8s-agents-operator/api/v1alpha2"
	newreliccomv1beta1 "github.com/newrelic/k8s-agents-operator/api/v1beta1"
	newreliccomv1beta2 "github.com/newrelic/k8s-agents-operator/api/v1beta2"
	newreliccomv1beta3 "github.com/newrelic/k8s-agents-operator/api/v1beta3"
	"github.com/newrelic/k8s-agents-operator/internal/autodetect"
	"github.com/newrelic/k8s-agents-operator/internal/config"
	"github.com/newrelic/k8s-agents-operator/internal/controller"
	"github.com/newrelic/k8s-agents-operator/internal/instrumentation"
	instrumentationupgrade "github.com/newrelic/k8s-agents-operator/internal/migrate/upgrade"
	"github.com/newrelic/k8s-agents-operator/internal/version"
	"github.com/newrelic/k8s-agents-operator/internal/webhook"
	// +kubebuilder:scaffold:imports

	certmgrv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmgrv1meta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
)

var healthCheckTickInterval = time.Second * 15
var setupLog = ctrl.Log.WithName("setup")

func getScheme() *k8sruntime.Scheme {
	var scheme = k8sruntime.NewScheme()
	sb := k8sruntime.NewSchemeBuilder(
		k8sscheme.AddToScheme,
		openshift_routev1.AddToScheme,
		newreliccomv1alpha2.AddToScheme,
		newreliccomv1beta1.AddToScheme,
		newreliccomv1beta2.AddToScheme,
		newreliccomv1beta3.AddToScheme,
	)
	utilruntime.Must(sb.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
	return scheme
}

type managerConfig struct {
	leaseDuration                 time.Duration
	renewDeadline                 time.Duration
	retryPeriod                   time.Duration
	webhookBindAddr               string
	webhookCertDir                string
	metricsBindAddr               string
	healthBindAddr                string
	enableHTTP2                   bool
	secureMetrics                 bool
	enableLeaderElection          bool
	leaderElectionID              string
	leaderElectionReleaseOnCancel bool
}

func getManagerOptions(scheme *k8sruntime.Scheme, cfg managerConfig) ctrl.Options {
	var webhookTlsOpts []func(*tls.Config)
	var metricsTlsOpts []func(*tls.Config)
	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(component string) func(c *tls.Config) {
		return func(c *tls.Config) {
			setupLog.Info("disabling http/2", "component", component)
			c.NextProtos = []string{"http/1.1"}
		}
	}
	if !cfg.enableHTTP2 {
		webhookTlsOpts = append(webhookTlsOpts, disableHTTP2("webhook"))
		metricsTlsOpts = append(webhookTlsOpts, disableHTTP2("metrics"))
	}
	webhookHost, webhookPort, err := net.SplitHostPort(cfg.webhookBindAddr)
	if err != nil {
		setupLog.Error(err, "invalid webhook bind address")
		os.Exit(1)
	}
	webhooksvcport, err := strconv.Atoi(webhookPort)
	if err != nil {
		setupLog.Error(err, "invalid webhook service port")
		os.Exit(1)
	}
	webhookServer := webhookruntime.NewServer(webhookruntime.Options{
		TLSOpts: webhookTlsOpts,
		Port:    webhooksvcport,
		Host:    webhookHost,
		CertDir: cfg.webhookCertDir,
	})
	metricsServerOptions := metricsserver.Options{
		BindAddress:   cfg.metricsBindAddr,
		SecureServing: cfg.secureMetrics,
		// TODO(user): TLSOpts is used to allow configuring the TLS config used for the server. If certificates are
		// not provided, self-signed certificates will be generated by default. This option is not recommended for
		// production environments as self-signed certificates do not offer the same level of trust and security
		// as certificates issued by a trusted Certificate Authority (CA). The primary risk is potentially allowing
		// unauthorized access to sensitive metrics data. Consider replacing with CertDir, CertName, and KeyName
		// to provide certificates, ensuring the server communicates using trusted and secure certificates.
		TLSOpts: metricsTlsOpts,
	}
	if cfg.secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: cfg.healthBindAddr,
		LeaderElection:         cfg.enableLeaderElection,
		LeaderElectionID:       cfg.leaderElectionID,
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
		LeaderElectionReleaseOnCancel: cfg.leaderElectionReleaseOnCancel,
		LeaseDuration:                 &cfg.leaseDuration,
		RenewDeadline:                 &cfg.renewDeadline,
		RetryPeriod:                   &cfg.retryPeriod,
	}
	return mgrOptions
}

type mainFlags struct {
	metricsAddr          string
	probeAddr            string
	webhooksvc           string
	enableLeaderElection bool
	secureMetrics        bool
	enableHTTP2          bool
}

func parseArgs() (mainFlags, zap.Options) {
	var flags mainFlags
	flag.StringVar(&flags.metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to.  Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&flags.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&flags.webhooksvc, "webhook-service-bind-address", ":9443", "The address the webhook service endpoint binds to.")
	flag.BoolVar(&flags.enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.  Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&flags.secureMetrics, "metrics-secure", true, "If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&flags.enableHTTP2, "enable-http2", true, "If set, HTTP/2 will be enabled for the metrics and webhook servers")

	zapOpts := zap.Options{}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	return flags, zapOpts
}

func main() {
	scheme := getScheme()
	flags, opts := parseArgs()

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)
	klog.SetLogger(logger)

	operatorNamespace := os.Getenv("OPERATOR_NAMESPACE")
	if operatorNamespace == "" {
		setupLog.Info("env var OPERATOR_NAMESPACE is required")
		os.Exit(1)
	}

	// Webhook configuration names from environment (set by Helm)
	webhookValidatingConfigName := os.Getenv("WEBHOOK_VALIDATING_CONFIG_NAME")
	webhookMutatingConfigName := os.Getenv("WEBHOOK_MUTATING_CONFIG_NAME")
	webhookServiceName := os.Getenv("WEBHOOK_SERVICE_NAME")
	webhookSecretName := os.Getenv("WEBHOOK_SECRET_NAME")
	certManagerEnabled := os.Getenv("CERT_MANAGER_ENABLED") == "true"
	certManagerCertName := os.Getenv("CERT_MANAGER_CERTIFICATE_NAME")

	if webhookValidatingConfigName == "" {
		setupLog.Info("env var WEBHOOK_VALIDATING_CONFIG_NAME is required")
		os.Exit(1)
	}
	if webhookMutatingConfigName == "" {
		setupLog.Info("env var WEBHOOK_MUTATING_CONFIG_NAME is required")
		os.Exit(1)
	}
	if webhookServiceName == "" {
		setupLog.Info("env var WEBHOOK_SERVICE_NAME is required")
		os.Exit(1)
	}
	if webhookSecretName == "" {
		setupLog.Info("env var WEBHOOK_SECRET_NAME is required")
		os.Exit(1)
	}
	if certManagerEnabled && certManagerCertName == "" {
		setupLog.Info("env var CERT_MANAGER_CERTIFICATE_NAME is required when cert-manager is enabled")
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

	// see https://github.com/openshift/library-go/blob/4362aa519714a4b62b00ab8318197ba2bba51cb7/pkg/config/leaderelection/leaderelection.go#L104
	leaseDuration := time.Second * 137
	renewDeadline := time.Second * 107
	retryPeriod := time.Second * 26
	mgrOptions := getManagerOptions(scheme, managerConfig{
		metricsBindAddr:               flags.metricsAddr,
		secureMetrics:                 flags.secureMetrics,
		webhookBindAddr:               flags.webhooksvc,
		healthBindAddr:                flags.probeAddr,
		enableLeaderElection:          flags.enableLeaderElection,
		leaderElectionID:              "9f7554c3.newrelic.com",
		leaderElectionReleaseOnCancel: true,
		leaseDuration:                 leaseDuration,
		renewDeadline:                 renewDeadline,
		retryPeriod:                   retryPeriod,
		enableHTTP2:                   flags.enableHTTP2,
	})

	watchNamespace, found := os.LookupEnv("WATCH_NAMESPACE")
	if found {
		setupLog.Info("watching namespace(s)", "namespaces", watchNamespace)
	} else {
		setupLog.Info("the env var WATCH_NAMESPACE isn't set, watching all namespaces")
	}
	if watchNamespace != "" {
		nsDefaults := map[string]cache.Config{}
		for _, watchNamespaceEntry := range strings.Split(watchNamespace, ",") {
			nsDefaults[watchNamespaceEntry] = cache.Config{}
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

	instrumentationStatusUpdater := instrumentation.NewInstrumentationStatusUpdater(mgr.GetClient())
	healthApi := instrumentation.NewHealthCheckApi(http.DefaultClient)
	healthMonitor := instrumentation.NewHealthMonitor(
		instrumentationStatusUpdater, healthApi, healthCheckTickInterval, 50, 50, 2,
	)
	go func() {
		<-ctx.Done()
		logger.Info("Shutting down health checker")
		stopCtx, stopCtxCancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer stopCtxCancel()
		if err = healthMonitor.Stop(stopCtx); err != nil {
			setupLog.Error(err, "failed to stop health checker")
		} else {
			logger.Info("Shut down health checker")
		}
	}()

	if err = setupReconcilers(mgr, healthMonitor, operatorNamespace); err != nil {
		setupLog.Error(err, "failed to setup reconcilers")
		os.Exit(1)
	}
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = setupWebhooks(mgr, operatorNamespace); err != nil {
			setupLog.Error(err, "failed to setup webhooks")
			os.Exit(1)
		}
	} else {
		ctrl.Log.Info("Webhooks are disabled, operator is running an unsupported mode", "ENABLE_WEBHOOKS", "false")
	}
	// +kubebuilder:scaffold:builder

	if err = registerApiHealth(mgr, operatorNamespace, webhookValidatingConfigName, webhookMutatingConfigName, webhookServiceName, webhookSecretName, certManagerEnabled, certManagerCertName); err != nil {
		setupLog.Error(err, "failed to register api healthz and readyz")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func infoHealthCheck(mgr manager.Manager, name string) func(*http.Request) error {
	return func(req *http.Request) error {
		mgr.GetLogger().Info("health check occurred", "name", name, "path", req.RequestURI)
		return nil
	}
}

// cachedHealthChecker caches health check results to avoid expensive API calls on every probe
type cachedHealthChecker struct {
	check    func(*http.Request) error
	cache    atomic.Value // stores *healthCheckResult
	cacheTTL time.Duration
	mu       sync.Mutex
}

type healthCheckResult struct {
	err       error
	timestamp time.Time
}

func (c *cachedHealthChecker) Check(req *http.Request) error {
	// Try cache first
	if cached := c.cache.Load(); cached != nil {
		result := cached.(*healthCheckResult)
		if time.Since(result.timestamp) < c.cacheTTL {
			return result.err
		}
	}

	// Cache miss or expired, do real check
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring lock
	if cached := c.cache.Load(); cached != nil {
		result := cached.(*healthCheckResult)
		if time.Since(result.timestamp) < c.cacheTTL {
			return result.err
		}
	}

	// Perform actual check
	err := c.check(req)
	c.cache.Store(&healthCheckResult{
		err:       err,
		timestamp: time.Now(),
	})
	return err
}

func registerApiHealth(mgr manager.Manager, operatorNamespace, webhookValidatingConfigName, webhookMutatingConfigName, webhookServiceName, webhookSecretName string, certManagerEnabled bool, certManagerCertName string) error {
	// Create cached health checker with 5-second TTL
	cachedChecker := &cachedHealthChecker{
		check:    WebhookReadyCheck("webhook-ready", mgr, webhookValidatingConfigName, webhookMutatingConfigName, operatorNamespace, webhookServiceName, webhookSecretName, certManagerEnabled, certManagerCertName),
		cacheTTL: 5 * time.Second,
	}

	if err := mgr.AddHealthzCheck("webhook-ready", cachedChecker.Check); err != nil {
		setupLog.Error(err, "unable to register webhook ready check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("ready", cachedChecker.Check); err != nil {
		return fmt.Errorf("unable to register ready check: %w", err)
	}

	return nil
}

func setupWebhooks(mgr manager.Manager, operatorNamespace string) error {
	if err := setupInstrumentationWebhooks(mgr, operatorNamespace); err != nil {
		return err
	}
	return setupPodMutationWebhook(mgr, operatorNamespace, ctrl.Log.WithName("mutation-webhook"))
}

func setupInstrumentationWebhooks(mgr manager.Manager, operatorNamespace string) error {
	type wehhookSetup struct {
		name  string
		setup func(mgr ctrl.Manager, operatorNamespace string) error
	}
	webhooks := []wehhookSetup{
		{name: "v1alpha2", setup: newreliccomv1alpha2.SetupWebhookWithManager},
		{name: "v1beta1", setup: newreliccomv1beta1.SetupWebhookWithManager},
		{name: "v1beta2", setup: newreliccomv1beta2.SetupWebhookWithManager},
		{name: "v1beta3", setup: newreliccomv1beta3.SetupWebhookWithManager},
	}
	for _, webhookEntry := range webhooks {
		if err := webhookEntry.setup(mgr, operatorNamespace); err != nil {
			return fmt.Errorf("unable to register %s.instrumentation webhook: %w", webhookEntry.name, err)
		}
	}
	return nil
}

func setupPodMutationWebhook(mgr manager.Manager, operatorNamespace string, logger logr.Logger) error {
	// Register the Pod mutation webhook
	if err := webhook.SetupWebhookWithManager(mgr, operatorNamespace, logger); err != nil {
		return fmt.Errorf("unable to register pod mutation webhook: %w", err)
	}
	return nil
}

func setupReconcilers(mgr manager.Manager, healthMonitor *instrumentation.HealthMonitor, operatorNamespace string) error {
	var err error
	if err = (&controller.NamespaceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, healthMonitor); err != nil {
		return fmt.Errorf("unable to create namespace controller: %w", err)
	}
	if err = (&controller.PodReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, healthMonitor, operatorNamespace); err != nil {
		return fmt.Errorf("unable to create pod controller: %w", err)
	}
	if err = (&controller.InstrumentationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, healthMonitor, operatorNamespace); err != nil {
		return fmt.Errorf("unable to create instrumentation controller: %w", err)
	}
	return nil
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

// WebhookReadyCheck returns an error if the webhook is not ready, marking the operator as not ready.
func WebhookReadyCheck(name string, mgr ctrl.Manager, webhookValidatingConfigName, webhookMutatingConfigName, webhookServiceNamespace, webhookServiceName, webhookSecretName string, certManagerEnabled bool, certManagerCertName string) func(req *http.Request) error {

	// Use the manager's client to access Kubernetes resources
	c := mgr.GetClient()

	return func(req *http.Request) (err error) {
		defer func() {
			if err != nil {
				mgr.GetLogger().Error(err, "health check failed")
			}
		}()
		setupLog.Info("health check occurred", "name", name, "path", req.RequestURI)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// NOTE: We intentionally do NOT check for service endpoint readiness here because
		// that would create a circular dependency - the pod won't be marked ready until
		// this readiness check passes, but we'd be checking if the service has ready endpoints.
		// The readiness of the webhook service is implicitly validated by the other checks.

		// 2. Check for TLS Secret Readiness (Self-Signed or cert-manager)
		// This ensures the certificate and key are present for the Pods to use.
		if err = checkTLSCertSecret(ctx, c, webhookServiceNamespace, webhookSecretName); err != nil {
			return fmt.Errorf("webhook TLS secret not ready: %w", err)
		}

		// 2b. Check for cert-manager Certificate readiness (if cert-manager is enabled)
		// Note: This check is optional and may fail if cert-manager CRDs are not installed
		if certManagerEnabled {
			if err = checkCertManagerCertificate(ctx, c, webhookServiceNamespace, certManagerCertName); err != nil {
				// Log but don't fail if cert-manager types aren't registered
				// This can happen if cert-manager isn't fully installed yet
				mgr.GetLogger().V(1).Info("cert-manager certificate check skipped", "error", err.Error())
			}
		}

		// 3. Check for CA Bundle Injection/Presence in Webhook Configuration
		// This ensures the API server will trust the webhook.
		if err = checkValidatingWebhookCABundleInjection(ctx, c, webhookValidatingConfigName); err != nil {
			return fmt.Errorf("webhook config CA bundle not ready: %w", err)
		}

		// 3. Check for CA Bundle Injection/Presence in Webhook Configuration
		// This ensures the API server will trust the webhook.
		if err = checkMutatingWebhookCABundleInjection(ctx, c, webhookMutatingConfigName); err != nil {
			return fmt.Errorf("webhook config CA bundle not ready: %w", err)
		}

		// If all checks pass, the webhook is ready!
		return nil
	}
}

//nolint:staticcheck // SA1019: Endpoints is deprecated but kept for backward compatibility
func checkServiceEndpoints(ctx context.Context, c client.Client, webhookServiceNamespace, webhookServiceName string) error {
	var endpoints corev1.Endpoints //nolint:staticcheck // SA1019: Endpoints is deprecated but kept for backward compatibility
	namespacedName := types.NamespacedName{Name: webhookServiceName, Namespace: webhookServiceNamespace}

	if err := c.Get(ctx, namespacedName, &endpoints); err != nil {
		return client.IgnoreNotFound(err) // If not found, it's not ready
	}

	// Check for at least one ready address in the subsets
	if len(endpoints.Subsets) > 0 && len(endpoints.Subsets[0].Addresses) > 0 {
		return nil // Endpoints are ready
	}
	return fmt.Errorf("no ready endpoints found for service %s/%s", webhookServiceNamespace, webhookServiceName)
}

// checkTLSCertSecret Check for TLS Secret readiness (based on self-signed scenario)
func checkTLSCertSecret(ctx context.Context, c client.Client, webhookServiceNamespace, webhookSecretName string) error {
	var tlsSecret corev1.Secret
	namespacedName := types.NamespacedName{Name: webhookSecretName, Namespace: webhookServiceNamespace}

	// First, check if the secret exists in Kubernetes API
	if err := c.Get(ctx, namespacedName, &tlsSecret); err != nil {
		return client.IgnoreNotFound(err)
	}

	if len(tlsSecret.Data[corev1.TLSCertKey]) == 0 || len(tlsSecret.Data[corev1.TLSPrivateKeyKey]) == 0 {
		return fmt.Errorf("tls secret found, but 'tls.crt' or 'tls.key' data is empty")
	}

	// Second, verify the certificates are actually available on the filesystem
	// This is critical because Kubernetes secret propagation to mounted volumes can take several seconds
	// The webhook server reads from the filesystem, not the K8s API
	certDir := "/tmp/k8s-webhook-server/serving-certs" // controller-runtime default
	certPath := filepath.Join(certDir, corev1.TLSCertKey)
	keyPath := filepath.Join(certDir, corev1.TLSPrivateKeyKey)

	// Check if certificate files exist and are readable
	if _, err := os.Stat(certPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("certificate file not yet available on filesystem: %s (secret propagation in progress)", certPath)
		}
		return fmt.Errorf("cannot access certificate file %s: %w", certPath, err)
	}

	if _, err := os.Stat(keyPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("private key file not yet available on filesystem: %s (secret propagation in progress)", keyPath)
		}
		return fmt.Errorf("cannot access private key file %s: %w", keyPath, err)
	}

	// Verify we can actually load and parse the certificate
	// This ensures the files aren't corrupted or incomplete during propagation
	certPEMBlock, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate file: %w", err)
	}
	keyPEMBlock, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key file: %w", err)
	}

	// Validate the certificate and key are properly formatted and match
	_, err = tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return fmt.Errorf("invalid TLS certificate/key pair on filesystem: %w", err)
	}

	return nil
}

// checkMutatingWebhookCABundleInjection Check for CA Bundle Injection/Presence
func checkMutatingWebhookCABundleInjection(ctx context.Context, c client.Client, webhookConfigName string) error {
	var mwhc admissionv1.MutatingWebhookConfiguration
	namespacedName := types.NamespacedName{Name: webhookConfigName} // WebhookConfig is cluster-scoped

	if err := c.Get(ctx, namespacedName, &mwhc); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("mutating webhook configuration %s not found", webhookConfigName)
		}
		return err
	}

	if len(mwhc.Webhooks) == 0 {
		return fmt.Errorf("mutating webhook configuration %s has no webhooks", webhookConfigName)
	}

	for _, webhook := range mwhc.Webhooks {
		if len(webhook.ClientConfig.CABundle) == 0 {
			return fmt.Errorf("mutating webhook %s is missing the CA bundle", webhook.Name)
		}
		// Validate it's a reasonable size for a certificate (basic sanity check)
		if len(webhook.ClientConfig.CABundle) < 100 {
			return fmt.Errorf("mutating webhook %s has suspiciously short CA bundle (%d bytes)", webhook.Name, len(webhook.ClientConfig.CABundle))
		}
	}
	return nil
}

// checkValidatingWebhookCABundleInjection Check for CA Bundle Injection/Presence
func checkValidatingWebhookCABundleInjection(ctx context.Context, c client.Client, webhookConfigName string) error {
	var vwhc admissionv1.ValidatingWebhookConfiguration
	namespacedName := types.NamespacedName{Name: webhookConfigName} // WebhookConfig is cluster-scoped

	if err := c.Get(ctx, namespacedName, &vwhc); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("validating webhook configuration %s not found", webhookConfigName)
		}
		return err
	}

	if len(vwhc.Webhooks) == 0 {
		return fmt.Errorf("validating webhook configuration %s has no webhooks", webhookConfigName)
	}

	for _, webhook := range vwhc.Webhooks {
		if len(webhook.ClientConfig.CABundle) == 0 {
			return fmt.Errorf("validating webhook %s is missing the CA bundle", webhook.Name)
		}
		// Validate it's a reasonable size for a certificate (basic sanity check)
		if len(webhook.ClientConfig.CABundle) < 100 {
			return fmt.Errorf("validating webhook %s has suspiciously short CA bundle (%d bytes)", webhook.Name, len(webhook.ClientConfig.CABundle))
		}
	}
	return nil
}

// checkServiceEndpointSlices Check for ready Service Endpoints using EndpointSlice
func checkServiceEndpointSlices(ctx context.Context, c client.Client, webhookServiceNamespace, webhookServiceName string) error {

	// Create a label selector to find all EndpointSlices for the target Service.
	// EndpointSlices are linked to Services via the "kubernetes.io/service-name" label.
	selector := labels.SelectorFromSet(labels.Set{
		"kubernetes.io/service-name": webhookServiceName,
	})

	listOptions := &client.ListOptions{
		Namespace:     webhookServiceNamespace,
		LabelSelector: selector,
	}

	var endpointSliceList discoveryv1.EndpointSliceList

	// List all EndpointSlices associated with the Service
	if err := c.List(ctx, &endpointSliceList, listOptions); err != nil {
		return fmt.Errorf("failed to list EndpointSlices for service %s/%s: %w", webhookServiceNamespace, webhookServiceName, err)
	}

	if len(endpointSliceList.Items) == 0 {
		return fmt.Errorf("no EndpointSlices found for service %s/%s (Service may not exist or Pods are not yet selected)", webhookServiceNamespace, webhookServiceName)
	}

	// Iterate through all EndpointSlices and check for ready endpoints
	var readyEndpoints int
	var servingEndpoints int
	for _, slice := range endpointSliceList.Items {
		for _, endpoint := range slice.Endpoints {

			// A key advantage of EndpointSlice: readiness is explicitly tracked via conditions.
			// We check the 'Ready' condition. A nil condition usually means true for compatibility,
			// but explicitly checking for true is safer for readiness.
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
				readyEndpoints += len(endpoint.Addresses)
			}
			if endpoint.Conditions.Serving != nil && *endpoint.Conditions.Serving {
				servingEndpoints += len(endpoint.Addresses)
			}
		}
	}

	if readyEndpoints == 0 {
		return fmt.Errorf("found %d EndpointSlices, but zero ready endpoints for service %s/%s", len(endpointSliceList.Items), webhookServiceNamespace, webhookServiceName)
	}

	ctrl.Log.Info("Found ready webhook endpoints", "count", readyEndpoints)
	ctrl.Log.Info("Found serving webhook endpoints", "count", servingEndpoints)
	return nil
}

// checkCertManagerCertificate Check for cert-manager Certificate readiness (optional, only used when cert-manager is enabled)
func checkCertManagerCertificate(ctx context.Context, c client.Client, namespace, certificateName string) error {
	certNamespacedName := types.NamespacedName{
		Name:      certificateName,
		Namespace: namespace,
	}

	var certificate certmgrv1.Certificate
	if err := c.Get(ctx, certNamespacedName, &certificate); err != nil {
		if errors.IsNotFound(err) {
			// Certificate resource not found, it hasn't been created yet.
			return fmt.Errorf("certificate resource %s/%s not found", namespace, certificateName)
		}
		return err // Other error
	}

	// Check the Certificate's Status Conditions
	for _, condition := range certificate.Status.Conditions {
		if condition.Type == certmgrv1.CertificateConditionReady {
			if condition.Status == certmgrv1meta.ConditionTrue {
				// The Certificate is Ready, meaning the backing Secret has been created and populated.
				return nil
			}
			// Certificate is not ready, check the reason for debugging/logging
			return fmt.Errorf("certificate %s is not ready: Reason: %s, Message: %s",
				certificateName, condition.Reason, condition.Message)
		}
	}

	// If the Ready condition is not present at all, assume not ready yet.
	return fmt.Errorf("certificate %s status conditions do not yet include Ready type", certificateName)
}
