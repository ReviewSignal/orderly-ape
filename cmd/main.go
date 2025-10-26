// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/joho/godotenv"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	// ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	ctrl "sigs.k8s.io/multicluster-runtime"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	// +kubebuilder:scaffold:imports

	"github.com/ReviewSignal/orderly-ape/internal/controller"
	"github.com/ReviewSignal/orderly-ape/internal/dex"
	"github.com/ReviewSignal/orderly-ape/internal/info"
	"github.com/ReviewSignal/orderly-ape/internal/jwt"
	"github.com/ReviewSignal/orderly-ape/internal/providers/worker"
	web "github.com/ReviewSignal/orderly-ape/internal/web/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	envFile  string
)

func init() {
	envFile = os.Getenv("ENV_FILE")
	if envFile != "" {
		err := godotenv.Load(envFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load environment file %s: %v\n", envFile, err)
			os.Exit(1)
		}
	} else {
		// Load the default .env file if no specific file is provided, but only if it exists
		if _, err := os.Stat(".env"); err == nil {
			envFile = ".env"
			err := godotenv.Load(".env")
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to load default environment file: %v\n", err)
				os.Exit(1)
			}
		}
	}

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(apev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

var Kubernetes kubeDetect

type kubeDetect struct {
	discoveryClient discovery.DiscoveryInterface
	APIGroups       []*metav1.APIGroup
	APIResourceList []*metav1.APIResourceList
}

func (d *kubeDetect) Configure(restConfig *rest.Config) error {
	var err error

	d.discoveryClient = kubernetes.NewForConfigOrDie(restConfig)
	d.APIGroups, d.APIResourceList, err = d.discoveryClient.ServerGroupsAndResources()

	return err
}

func (d *kubeDetect) HasGVK(gvk schema.GroupVersionKind) bool {
	for _, group := range d.APIResourceList {
		if group.GroupVersion == gvk.GroupVersion().String() {
			for _, resource := range group.APIResources {
				if resource.Kind == gvk.Kind {
					return true
				}
			}
		}
	}
	return false
}

func gkeLogEncodingConfig(ec *zapcore.EncoderConfig) {
	ec.MessageKey = "message"
	ec.LevelKey = "severity"
	ec.TimeKey = "time"
	ec.StacktraceKey = "stack_trace"
}

// createGrafanaClient creates a Grafana client if the API URL and token are provided
func createGrafanaClient(apiURL, token string) controller.GrafanaClient {
	if apiURL == "" || token == "" {
		setupLog.Info("Grafana client not configured, skipping Grafana integration")
		return nil
	}
	return controller.NewGrafanaClient(apiURL, token)
}

// parseGrafanaOrgID parses the Grafana organization ID from a string
func parseGrafanaOrgID(orgID string) int {
	if orgID == "" {
		return 1
	}
	id, err := strconv.Atoi(orgID)
	if err != nil {
		setupLog.Error(err, "failed to parse Grafana organization ID, using default", "orgID", orgID)
		return 1
	}
	return id
}

// nolint:gocyclo,lll
func main() {
	var devMode bool
	var webappAddr string
	var baseURL string
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	var oidcIssuerURL string
	var oidcClientID string
	var oidcClientSecret string
	var oidcRedirectURL string
	var accessLogFile string
	var accessLogFormat string
	var grafanaURL string
	var grafanaToken string
	var grafanaPublicOrgID string
	var jwtPrivateKeyPath string
	var jwtPublicKeyPath string
	var jwtIssuer string
	var victoriaLogsURL string
	var victoriaLogsInsertURL string
	var victoriaLogsAccountID int32
	var defaultScenarioCPU string
	var defaultScenarioMemory string
	var dexGRPCEndpoint string
	var dexGRPCInsecure bool

	flag.BoolVar(&devMode, "dev", false, "Enable development mode This changes some default, like logging and manager behavior.")
	flag.StringVar(&webappAddr, "webapp-addr", ":8000", "The address the webapp is started on.")
	flag.StringVar(&baseURL, "base-url", "", "The base URL for the webapp. If not set, it will be inferrd from webapp addri (ie. localhost:8000).")
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&oidcIssuerURL, "oidc-issuer-url", "", "The URL of the OIDC issuer.")
	flag.StringVar(&oidcClientID, "oidc-client-id", "", "The client ID for the OIDC client.")
	flag.StringVar(&oidcClientSecret, "oidc-client-secret", "", "The client secret for the OIDC client.")
	flag.StringVar(&oidcRedirectURL, "oidc-redirect-url", "", "The redirect URL for the OIDC client. (default: \"http://localhost:8000\")")
	flag.StringVar(&accessLogFile, "access-log-file", "", "The path to the access log file. If empty, access logs are not written.")
	flag.StringVar(&accessLogFormat, "access-log-format", "", "The access log format. Options: \"combined\", or \"json\". (default: \"combined\").")
	flag.StringVar(&grafanaURL, "grafana-url", "", "The Grafana API address. (default: http://localhost:3000)")
	flag.StringVar(&grafanaToken, "grafana-token", "", "The Grafana API token.")
	flag.StringVar(&grafanaPublicOrgID, "grafana-public-org-id", "", "The Grafana organization ID for public dashboards (default: \"1\").")
	flag.StringVar(&jwtPrivateKeyPath, "jwt-private-key-path", "", "The path to the JWT private key file (PEM format).")
	flag.StringVar(&jwtPublicKeyPath, "jwt-public-key-path", "", "The path to the JWT public key file (PEM format).")
	flag.StringVar(&jwtIssuer, "jwt-issuer", "", "The JWT issuer URL.")
	flag.StringVar(&victoriaLogsURL, "victorialogs-url", "", "The Victoria Logs API address.")
	flag.StringVar(&victoriaLogsInsertURL, "victorialogs-insert-url", "", "The Victoria Logs Insert API address.")
	var victoriaLogsAccountIDInt int
	flag.IntVar(&victoriaLogsAccountIDInt, "victorialogs-account-id", 0, "The Victoria Logs Account ID (default: 0).")
	flag.StringVar(&defaultScenarioCPU, "default-scenario-cpu", "1", "Default CPU limit for test scenarios (default: \"1\").")
	flag.StringVar(&defaultScenarioMemory, "default-scenario-memory", "2Gi", "Default memory limit for test scenarios (default: \"2Gi\").")
	flag.StringVar(&dexGRPCEndpoint, "dex-grpc-endpoint", "", "The DEX gRPC API endpoint for user management. If set, users are managed via DEX instead of Kubernetes CRDs.")
	flag.BoolVar(&dexGRPCInsecure, "dex-grpc-insecure", false, "If set, the DEX gRPC connection is not secured via TLS.")

	var err error
	opts := zap.Options{
		Development: true,
		// Remove unusable controller-runtime stack traces https://github.com/kubernetes-sigs/kubebuilder/issues/1593
		StacktraceLevel: zapcore.DPanicLevel,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if webappAddr == ":8000" && os.Getenv("PORT") != "" {
		webappAddr = ":" + os.Getenv("PORT")
	}

	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
	}

	if baseURL == "" {
		parts := strings.SplitN(webappAddr, ":", 2)
		if len(parts) == 1 {
			baseURL = "http://" + webappAddr
		} else if parts[0] == "" {
			baseURL = "http://localhost:" + parts[1]
		} else {
			baseURL = "http://" + webappAddr
		}
	}

	if oidcIssuerURL == "" {
		oidcIssuerURL = os.Getenv("OIDC_ISSUER_URL")
	}
	if oidcClientID == "" {
		oidcClientID = os.Getenv("OIDC_CLIENT_ID")
	}
	if oidcClientSecret == "" {
		oidcClientSecret = os.Getenv("OIDC_CLIENT_SECRET")
	}
	if oidcRedirectURL == "" {
		oidcRedirectURL = os.Getenv("OIDC_REDIRECT_URL")
	}

	if grafanaURL == "" {
		grafanaURL = os.Getenv("GRAFANA_URL")
		if grafanaURL == "" {
			grafanaURL = "http://localhost:3000"
		}
	}
	if grafanaToken == "" {
		grafanaToken = os.Getenv("GRAFANA_TOKEN")
	}
	if grafanaPublicOrgID == "" {
		grafanaPublicOrgID = os.Getenv("GRAFANA_PUBLIC_ORG_ID")
		if grafanaPublicOrgID == "" {
			grafanaPublicOrgID = "1"
		}
	}

	if jwtPrivateKeyPath == "" {
		jwtPrivateKeyPath = os.Getenv("JWT_PRIVATE_KEY_PATH")
	}
	if jwtPublicKeyPath == "" {
		jwtPublicKeyPath = os.Getenv("JWT_PUBLIC_KEY_PATH")
	}
	if jwtIssuer == "" {
		jwtIssuer = os.Getenv("JWT_ISSUER")
	}
	if jwtIssuer == "" {
		jwtIssuer = baseURL
	}

	if victoriaLogsURL == "" {
		victoriaLogsURL = os.Getenv("VICTORIALOGS_URL")
	}
	if victoriaLogsInsertURL == "" {
		victoriaLogsInsertURL = os.Getenv("VICTORIALOGS_INSERT_URL")
	}
	if victoriaLogsInsertURL == "" {
		if victoriaLogsURL == "" {
			victoriaLogsInsertURL, err = url.JoinPath(baseURL, "/victorialogs/insert")
			if err != nil {
				setupLog.Error(err, "failed to parse Victoria Logs insert URL")
				os.Exit(1)
			}
		} else {
			victoriaLogsInsertURL, err = url.JoinPath(victoriaLogsURL, "/insert")
			if err != nil {
				setupLog.Error(err, "failed to parse Victoria Logs insert URL")
				os.Exit(1)
			}
		}
	}
	victoriaLogsUseProxiedInsert := strings.HasPrefix(victoriaLogsInsertURL, baseURL)
	victoriaLogsAccountID = int32(victoriaLogsAccountIDInt)

	if dexGRPCEndpoint == "" {
		dexGRPCEndpoint = os.Getenv("DEX_GRPC_ENDPOINT")
	}

	if dexGRPCInsecureEnv := os.Getenv("DEX_GRPC_INSECURE"); dexGRPCInsecureEnv != "" {
		dexGRPCInsecure = dexGRPCInsecureEnv == "true" || dexGRPCInsecureEnv == "1"
	}

	if grafanaURL != "" {
		info.GrafanaEnabled = true
	}

	if accessLogFormat == "" {
		accessLogFormat = "combined"
		if info.GCP.GKEClusterName != "" {
			accessLogFormat = "json"
		}
	}
	if info.GCP.GKEClusterName != "" {
		opts.EncoderConfigOptions = append(opts.EncoderConfigOptions, gkeLogEncodingConfig)
	}

	ctrlNamespace := info.PodNamespace()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog.Info("starting Orderly Ape operator", "version", info.Version.GitVersion, "env", envFile, "controller-namespace", ctrlNamespace, "base-url", baseURL)
	signalCtx := ctrl.SetupSignalHandler()
	setupCtx, cancel := context.WithCancel(signalCtx)
	defer cancel()

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

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.4/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.4/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	restConfig := ctrl.GetConfigOrDie()
	err = Kubernetes.Configure(restConfig)
	if err != nil {
		setupLog.Error(err, "unable to get API resources")
		os.Exit(1)
	}

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "81l298ke.ape.reviewsignal.com",
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
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				ctrlNamespace: {},
			},
		},
	}
	if devMode {
		noTimeout := 0 * time.Second
		mgrOptions.GracefulShutdownTimeout = &noTimeout // disable graceful shutdown in dev mode
	}

	jwtManager, err := jwt.NewManager(jwt.Config{
		PrivateKeyPath: jwtPrivateKeyPath,
		PublicKeyPath:  jwtPublicKeyPath,
		Issuer:         jwtIssuer,
	})
	if err != nil {
		setupLog.Error(err, "unable to create JWT manager")
		os.Exit(1)
	}

	provider := worker.New(worker.Options{
		Namespace: ctrlNamespace,
		ClusterOptions: []cluster.Option{
			func(clusterOptions *cluster.Options) {
				clusterOptions.Scheme = scheme
			},
		},
	})
	mgr, err := ctrl.NewManager(restConfig, provider, mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	if err = provider.SetupWithManager(setupCtx, mgr); err != nil {
		setupLog.Error(err, "unable to setup worker provider with manager")
		os.Exit(1)
	}

	localMgr := mgr.GetLocalManager()

	if err = (&controller.InfluxDBReconciler{
		Client:        localMgr.GetClient(),
		Scheme:        localMgr.GetScheme(),
		GrafanaClient: createGrafanaClient(grafanaURL, grafanaToken),
		GrafanaOrgID:  parseGrafanaOrgID(grafanaPublicOrgID),
	}).SetupWithManager(localMgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "InfluxDB")
		os.Exit(1)
	}

	if err = (&controller.TestRunReconciler{
		Client:                localMgr.GetClient(),
		Scheme:                localMgr.GetScheme(),
		GrafanaClient:         createGrafanaClient(grafanaURL, grafanaToken),
		GrafanaURL:            grafanaURL,
		VictoriaLogsURL:       victoriaLogsURL,
		VictoriaLogsAccountID: victoriaLogsAccountID,
	}).SetupWithManager(localMgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TestRun")
		os.Exit(1)
	}

	if err = (&controller.TestRunWorkerReconciler{
		Client:                    localMgr.GetClient(),
		Scheme:                    localMgr.GetScheme(),
		LocalNamespace:            ctrlNamespace,
		ProxiedVictoriaLogsInsert: victoriaLogsUseProxiedInsert,
		VictoriaLogsInsertURL:     victoriaLogsInsertURL,
		JWTManager:                jwtManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TestRunWorker")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := localMgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := localMgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	var dexClient *dex.Client
	if dexGRPCInsecure {
		dexClient, err = dex.NewInsecureClient(dexGRPCEndpoint)
		if err != nil {
			setupLog.Error(err, "unable to create DEX gRPC client")
			os.Exit(1)
		}
	} else {
		dexClient, err = dex.NewClient(dexGRPCEndpoint)
		if err != nil {
			setupLog.Error(err, "unable to create DEX gRPC client")
			os.Exit(1)
		}
	}

	if webappAddr != "" && webappAddr != "0" {
		setupLog.Info("Adding webapp server to manager")
		if err := (&web.Server{
			Addr:                  webappAddr,
			OIDCIssuerURL:         oidcIssuerURL,
			ClientID:              oidcClientID,
			ClientSecret:          oidcClientSecret,
			OAuth2RedirectURL:     oidcRedirectURL,
			AccessLogFilePath:     accessLogFile,
			AccessLogFormat:       accessLogFormat,
			ControllerNamespace:   ctrlNamespace,
			GrafanaURL:            grafanaURL,
			VictoriaLogsURL:       victoriaLogsURL,
			JWTManager:            jwtManager,
			DefaultScenarioCPU:    defaultScenarioCPU,
			DefaultScenarioMemory: defaultScenarioMemory,
			DexClient:             dexClient,
		}).SetupWithManager(localMgr); err != nil {
			setupLog.Error(err, "unable to setup webapp server")
			os.Exit(1)
		}
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
	if err := mgr.Start(signalCtx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
