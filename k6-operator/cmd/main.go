//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2024 ReviewSignal

package main

import (
	"crypto/tls"
	"flag"
	"os"
	"path"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	kmeta "kmodules.xyz/client-go/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting"
	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/client"
	"github.com/ReviewSignal/loadtesting/k6-operator/internal/options"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

// reads a value from the file /run/secrets/<name>
func fromSecretFile(name string) string {
	file := path.Join("/run/secrets", name)
	data, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		setupLog.Error(err, "unable to read secret file", "file", file)
		return ""
	}

	return string(data)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool

	var loadtestingRegion string
	var loadtestingAPIEndpoint string
	var loadtestingAPIUser string
	var loadtestingAPIPassword string
	var jobNamespace string

	flag.StringVar(&loadtestingAPIEndpoint, "loadtesting-api-endpoint", "", "The API endpoint for controlling the k6 load testing.")
	flag.StringVar(&loadtestingAPIUser, "loadtesting-api-user", "", "The API user for controlling the k6 load testing.")
	flag.StringVar(&loadtestingAPIPassword, "loadtesting-api-password", "", "The API password for controlling the k6 load testing.")
	flag.StringVar(&loadtestingRegion, "loadtesting-region", "", "The region this controller is running in. Required.")
	flag.StringVar(&jobNamespace, "job-namespace", "", "The namespace to create the k6 jobs in. Defaults to the namespace the controller is running in.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
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

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	if loadtestingRegion == "" {
		loadtestingRegion = fromSecretFile("REGION")
	}
	if loadtestingRegion == "" {
		setupLog.Error(nil, "loadtesting-region is required")
		os.Exit(1)
	}
	options.Region = loadtestingRegion

	if jobNamespace == "" {
		jobNamespace = fromSecretFile("JOBS_NAMESPACE")
	}
	if jobNamespace == "" {
		jobNamespace = kmeta.PodNamespace()
	}
	options.JobNamespace = jobNamespace

	if loadtestingAPIEndpoint == "" {
		loadtestingAPIEndpoint = fromSecretFile("API_ENDPOINT")
	}
	if loadtestingAPIEndpoint != "" {
		options.APIEndpoint = loadtestingAPIEndpoint
	}

	if loadtestingAPIUser == "" {
		loadtestingAPIUser = fromSecretFile("API_USER")
	}
	if loadtestingAPIUser != "" {
		options.APIUser = loadtestingAPIUser
	}

	if loadtestingAPIPassword == "" {
		loadtestingAPIPassword = fromSecretFile("API_PASSWORD")
	}
	if loadtestingAPIPassword != "" {
		options.APIPassword = loadtestingAPIPassword
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				jobNamespace: {},
			},
		},
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "32cf8cb2.reviewsignal.com",
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
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	apiClient, err := client.NewUncachedClient(
		client.WithBaseURL(options.APIEndpoint),
		client.WithUsername(options.APIUser),
		client.WithPassword(options.APIPassword),
		client.WithRegion(options.Region),
		client.WithLogger(ctrl.Log.WithName("loadtesting-client")),
	)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Deployment")
		os.Exit(1)
	}

	if err = (&controller.TestRunReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		APIClient: apiClient,
		Location:  options.Region,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TestRun")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	pinger, err := loadtesting.NewPinger(apiClient)
	if err != nil {
		setupLog.Error(err, "unable to set up pinger")
		os.Exit(1)
	}
	if err = mgr.Add(pinger); err != nil {
		setupLog.Error(err, "unable to set up pinger")
		os.Exit(1)

	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
