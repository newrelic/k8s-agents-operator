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
	"os"
	"runtime"
	"strings"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	k8sapiflag "k8s.io/component-base/cli/flag"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	v1alpha1 "github.com/newrelic-experimental/newrelic-agent-operator/api/v1alpha1"
	"github.com/newrelic-experimental/newrelic-agent-operator/internal/config"
	"github.com/newrelic-experimental/newrelic-agent-operator/internal/version"
	"github.com/newrelic-experimental/newrelic-agent-operator/internal/webhookhandler"
	"github.com/newrelic-experimental/newrelic-agent-operator/pkg/autodetect"
	"github.com/newrelic-experimental/newrelic-agent-operator/pkg/instrumentation"
	instrumentationupgrade "github.com/newrelic-experimental/newrelic-agent-operator/pkg/instrumentation/upgrade"
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

	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	// registers any flags that underlying libraries might use
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	v := version.Get()

	// add flags related to this operator
	var (
		metricsAddr               string
		probeAddr                 string
		enableLeaderElection      bool
		autoInstrumentationJava   string
		autoInstrumentationNodeJS string
		autoInstrumentationPython string
		autoInstrumentationDotNet string
		autoInstrumentationPhp    string
		autoInstrumentationGo     string
		labelsFilter              []string
		webhookPort               int
		tlsOpt                    tlsConfig
	)

	pflag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-addr", ":8081", "The address the probe endpoint binds to.")
	pflag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.StringVar(&autoInstrumentationJava, "auto-instrumentation-java-image", fmt.Sprintf("ghcr.io/newrelic-experimental/newrelic-agent-operator/instrumentation-java:%s", v.AutoInstrumentationJava), "The default New Relic Java instrumentation image. This image is used when no image is specified in the CustomResource.")
	pflag.StringVar(&autoInstrumentationNodeJS, "auto-instrumentation-nodejs-image", fmt.Sprintf("ghcr.io/newrelic-experimental/newrelic-agent-operator/instrumentation-nodejs:%s", v.AutoInstrumentationNodeJS), "The default New Relic NodeJS instrumentation image. This image is used when no image is specified in the CustomResource.")
	pflag.StringVar(&autoInstrumentationPython, "auto-instrumentation-python-image", fmt.Sprintf("ghcr.io/newrelic-experimental/newrelic-agent-operator/instrumentation-python:%s", v.AutoInstrumentationPython), "The default New Relic Python instrumentation image. This image is used when no image is specified in the CustomResource.")
	pflag.StringVar(&autoInstrumentationDotNet, "auto-instrumentation-dotnet-image", fmt.Sprintf("ghcr.io/newrelic-experimental/newrelic-agent-operator/instrumentation-dotnet:%s", v.AutoInstrumentationDotNet), "The default New Relic DotNet instrumentation image. This image is used when no image is specified in the CustomResource.")
	pflag.StringVar(&autoInstrumentationPhp, "auto-instrumentation-php-image", fmt.Sprintf("ghcr.io/newrelic-experimental/newrelic-agent-operator/instrumentation-php:%s", v.AutoInstrumentationDotNet), "The default New Relic Php instrumentation image. This image is used when no image is specified in the CustomResource.")
	pflag.StringVar(&autoInstrumentationGo, "auto-instrumentation-go-image", fmt.Sprintf("ghcr.io/open-telemetry/opentelemetry-go-instrumentation/autoinstrumentation-go:%s", v.AutoInstrumentationGo), "The default Opentelemtry Go instrumentation image. This image is used when no image is specified in the CustomResource.")

	pflag.StringArrayVar(&labelsFilter, "labels", []string{}, "Labels to filter away from propagating onto deploys")
	pflag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook endpoint binds to.")
	pflag.StringVar(&tlsOpt.minVersion, "tls-min-version", "VersionTLS12", "Minimum TLS version supported. Value must match version names from https://golang.org/pkg/crypto/tls/#pkg-constants.")
	pflag.StringSliceVar(&tlsOpt.cipherSuites, "tls-cipher-suites", nil, "Comma-separated list of cipher suites for the server. Values are from tls package constants (https://golang.org/pkg/crypto/tls/#pkg-constants). If omitted, the default Go cipher suites will be used")
	pflag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)

	logger.Info("Starting the New Relic Agent Operator",
		"newrelic-agent-operator", v.Operator,
		"auto-instrumentation-java", autoInstrumentationJava,
		"auto-instrumentation-nodejs", autoInstrumentationNodeJS,
		"auto-instrumentation-python", autoInstrumentationPython,
		"auto-instrumentation-dotnet", autoInstrumentationDotNet,
		"auto-instrumentation-php", autoInstrumentationPhp,
		"auto-instrumentation-go", autoInstrumentationGo,
		"build-date", v.BuildDate,
		"go-version", v.Go,
		"go-arch", runtime.GOARCH,
		"go-os", runtime.GOOS,
		"labels-filter", labelsFilter,
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
		config.WithAutoInstrumentationJavaImage(autoInstrumentationJava),
		config.WithAutoInstrumentationNodeJSImage(autoInstrumentationNodeJS),
		config.WithAutoInstrumentationPythonImage(autoInstrumentationPython),
		config.WithAutoInstrumentationDotNetImage(autoInstrumentationDotNet),
		config.WithAutoInstrumentationPhpImage(autoInstrumentationPhp),
		config.WithAutoInstrumentationGoImage(autoInstrumentationGo),
		config.WithAutoDetect(ad),
		config.WithLabelFilters(labelsFilter),
	)

	watchNamespace, found := os.LookupEnv("WATCH_NAMESPACE")
	if found {
		setupLog.Info("watching namespace(s)", "namespaces", watchNamespace)
	} else {
		setupLog.Info("the env var WATCH_NAMESPACE isn't set, watching all namespaces")
	}

	// see https://github.com/openshift/library-go/blob/4362aa519714a4b62b00ab8318197ba2bba51cb7/pkg/config/leaderelection/leaderelection.go#L104
	leaseDuration := time.Second * 137
	renewDeadline := time.Second * 107
	retryPeriod := time.Second * 26

	optionsTlSOptsFuncs := []func(*tls.Config){
		func(config *tls.Config) { tlsConfigSetting(config, tlsOpt) },
	}

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   webhookPort,
		TLSOpts:                optionsTlSOptsFuncs,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "9f7554c3.newrelic.com",
		Namespace:              watchNamespace,
		LeaseDuration:          &leaseDuration,
		RenewDeadline:          &renewDeadline,
		RetryPeriod:            &retryPeriod,
	}

	if strings.Contains(watchNamespace, ",") {
		mgrOptions.Namespace = ""
		mgrOptions.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(watchNamespace, ","))
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
	err = addDependencies(ctx, mgr, cfg, v)
	if err != nil {
		setupLog.Error(err, "failed to add/run bootstrap dependencies to the controller manager")
		os.Exit(1)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&v1alpha1.Instrumentation{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					v1alpha1.AnnotationDefaultAutoInstrumentationJava:   autoInstrumentationJava,
					v1alpha1.AnnotationDefaultAutoInstrumentationNodeJS: autoInstrumentationNodeJS,
					v1alpha1.AnnotationDefaultAutoInstrumentationPython: autoInstrumentationPython,
					v1alpha1.AnnotationDefaultAutoInstrumentationDotNet: autoInstrumentationDotNet,
					v1alpha1.AnnotationDefaultAutoInstrumentationPhp:    autoInstrumentationPhp,
					v1alpha1.AnnotationDefaultAutoInstrumentationGo:     autoInstrumentationGo,
				},
			},
		}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Instrumentation")
			os.Exit(1)
		}

		mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{
			Handler: webhookhandler.NewWebhookHandler(cfg, ctrl.Log.WithName("pod-webhook"), mgr.GetClient(),
				[]webhookhandler.PodMutator{
					instrumentation.NewMutator(logger, mgr.GetClient()),
				}),
		})
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

func addDependencies(_ context.Context, mgr ctrl.Manager, cfg config.Config, v version.Version) error {
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
			Logger:                ctrl.Log.WithName("instrumentation-upgrade"),
			DefaultAutoInstJava:   cfg.AutoInstrumentationJavaImage(),
			DefaultAutoInstNodeJS: cfg.AutoInstrumentationNodeJSImage(),
			DefaultAutoInstPython: cfg.AutoInstrumentationPythonImage(),
			DefaultAutoInstDotNet: cfg.AutoInstrumentationDotNetImage(),
			DefaultAutoInstPhp:    cfg.AutoInstrumentationPhpImage(),
			DefaultAutoInstGo:     cfg.AutoInstrumentationGoImage(),
			Client:                mgr.GetClient(),
		}
		return u.ManagedInstances(c)
	}))
	if err != nil {
		return fmt.Errorf("failed to upgrade Instrumentation instances: %w", err)
	}
	return nil
}

// This function get the option from command argument (tlsConfig), check the validity through k8sapiflag
// and set the config for webhook server.
// refer to https://pkg.go.dev/k8s.io/component-base/cli/flag
func tlsConfigSetting(cfg *tls.Config, tlsOpt tlsConfig) {
	// TLSVersion helper function returns the TLS Version ID for the version name passed.
	version, err := k8sapiflag.TLSVersion(tlsOpt.minVersion)
	if err != nil {
		setupLog.Error(err, "TLS version invalid")
	}
	cfg.MinVersion = version

	// TLSCipherSuites helper function returns a list of cipher suite IDs from the cipher suite names passed.
	cipherSuiteIDs, err := k8sapiflag.TLSCipherSuites(tlsOpt.cipherSuites)
	if err != nil {
		setupLog.Error(err, "Failed to convert TLS cipher suite name to ID")
	}
	cfg.CipherSuites = cipherSuiteIDs
}
