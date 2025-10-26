// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	logr "github.com/go-logr/logr"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/ReviewSignal/orderly-ape/internal/dex"
	"github.com/ReviewSignal/orderly-ape/internal/jwt"
	"github.com/ReviewSignal/orderly-ape/internal/web/auth"
	"github.com/ReviewSignal/orderly-ape/internal/web/grafana"
	"github.com/ReviewSignal/orderly-ape/internal/web/influxdb"
	"github.com/ReviewSignal/orderly-ape/internal/web/testruns"
	"github.com/ReviewSignal/orderly-ape/internal/web/testscenarios"
	"github.com/ReviewSignal/orderly-ape/internal/web/users"
	"github.com/ReviewSignal/orderly-ape/internal/web/victorialogs"
	"github.com/ReviewSignal/orderly-ape/internal/web/workers"
	"github.com/ReviewSignal/orderly-ape/web/utils/url"
)

type ServerInfo struct {
	GCPProjectID   string
	GKEClusterName string
}

type ServerInfoContextKey string

var ServerInfoKey = ServerInfoContextKey("ServerInfo")

type Server struct {
	Addr                  string
	OIDCIssuerURL         string
	ClientID              string
	ClientSecret          string
	OAuth2RedirectURL     string
	ControllerNamespace   string
	AccessLogFilePath     string
	AccessLogFormat       string
	GrafanaURL            string
	JWTManager            *jwt.Manager
	VictoriaLogsURL       string
	DefaultScenarioCPU    string
	DefaultScenarioMemory string
	DexClient             *dex.Client

	ServerInfo ServerInfo

	oidcProvider *oidc.Provider

	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

func (s *Server) Println(args ...any) {
	s.log.Info(fmt.Sprint(args...))
}

func (s *Server) SetupWithManager(mgr ctrl.Manager) error {
	var err error
	s.log = mgr.GetLogger().WithName("webapp")

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, s.OIDCIssuerURL)
	if err != nil {
		return err
	}
	s.oidcProvider = provider
	s.client = mgr.GetClient()
	s.scheme = mgr.GetScheme()

	server := &manager.Server{
		Name:   "webapp",
		Server: s.NewServer(),
	}

	return mgr.Add(server)
}

// nolint:unused,unusedfunc // this middelware is used for local development
func (s *Server) templErrorHandler(_ *http.Request, err error) http.Handler {
	s.log.Error(err, "error rendering template")
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	})
}

func (s *Server) injectLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(log.IntoContext(r.Context(), s.log)))
	})
}

func (s *Server) injectServerInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, ServerInfoKey, s.ServerInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) accessLogger(next http.Handler) http.Handler {
	var out io.Writer

	switch s.AccessLogFilePath {
	case "/dev/stdout":
		out = os.Stdout
	case "/dev/stderr":
		out = os.Stderr
	case "":
	case "/dev/null":
		break
	default:
		var err error
		out, err = os.OpenFile(s.AccessLogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if out == nil {
			next.ServeHTTP(w, r)
		} else {
			switch s.AccessLogFormat {
			case "json":
				handlers.CustomLoggingHandler(out, next, func(writer io.Writer, params handlers.LogFormatterParams) {
					l := map[string]any{
						"severity": "INFO",
						"time":     params.TimeStamp.Format(time.RFC3339Nano),
						"httpRequest": map[string]any{
							"requestMethod": params.Request.Method,
							"protocol":      params.Request.Proto,
							"status":        params.StatusCode,
							"requestUrl":    params.Request.URL.String(),
							"latency":       fmt.Sprintf("%v", time.Since(params.TimeStamp)),
							"responseSize":  params.Size,
							"userAgent":     params.Request.UserAgent(),
							"referer":       params.Request.Referer(),
							"requestSize":   params.Request.ContentLength,
							"remoteIp":      params.Request.RemoteAddr,
						},
					}

					line, err := json.Marshal(l)
					if err != nil {
						panic(err)
					}
					line = append(line, '\n')
					_, _ = writer.Write(line)
				}).ServeHTTP(w, r)
			default:
				handlers.CombinedLoggingHandler(out, next).ServeHTTP(w, r)
			}
		}
	})
}

func (s *Server) NewServer() *http.Server {
	a := auth.NewAuth(auth.Auth{
		ClientID:            s.ClientID,
		ClientSecret:        s.ClientSecret,
		OAuth2RedirectURL:   s.OAuth2RedirectURL,
		OIDCProvider:        s.oidcProvider,
		Client:              s.client,
		ControllerNamespace: s.ControllerNamespace,
	})

	r := mux.NewRouter()
	r.Use(s.injectLogger)
	r.Use(handlers.RecoveryHandler(handlers.RecoveryLogger(s), handlers.PrintRecoveryStack(true)))
	r.Use(s.accessLogger)
	r.Use(a.InjectUserMiddleware)

	r.Use(s.injectServerInfo)
	r.Use(url.Middleware(r))

	// Publicly accessible urls
	r.Path("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() == "GoogleHC/1.0" || strings.HasPrefix(r.UserAgent(), "kube-probe/") {
			w.WriteHeader(http.StatusOK)
			w.Header().Add("Content-Type", "text/plain")
			w.Header().Add("Content-Length", "2")
			_, _ = w.Write([]byte("OK"))
			return
		}
		http.RedirectHandler("/test-runs", http.StatusSeeOther).ServeHTTP(w, r)
	}).Name("root")
	r.Path("/favicon.ico").Handler(http.FileServer(http.Dir("public"))).Name("public:favicon")
	r.PathPrefix("/public/").Handler(http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))
	r.Path("/auth/callback").HandlerFunc(a.AuthCallback).Name("auth:callback")
	r.Path("/auth/logout").HandlerFunc(a.LogoutCallback).Name("auth:logout")

	// JWT endpoints (public)
	if s.JWTManager != nil {
		jwtHandler := jwt.NewHandler(s.JWTManager)
		r.Path("/.well-known/jwks.json").HandlerFunc(jwtHandler.HandleJWKS).Name("jwt:jwks")
		r.Path("/auth/verify-token").HandlerFunc(jwtHandler.HandleVerify).Name("auth:verify-token")
	}

	// Grafana proxy routes (public)
	if s.GrafanaURL != "" {
		grafanaProxy, err := grafana.NewProxyHandler(s.GrafanaURL)
		if err != nil {
			s.log.Error(err, "failed to create Grafana proxy handler")
		} else {
			// Dashboard routes with kiosk enforcement
			r.PathPrefix("/grafana/").Handler(grafanaProxy.Middleware(http.HandlerFunc(grafanaProxy.DashboardHandler)))
			r.PathPrefix("/results/{testid}").Handler(grafana.DashboardHandler(s.client, s.ControllerNamespace))
		}
	}

	// Victoria Logs proxy routes (JWT protected)
	if s.VictoriaLogsURL != "" && s.JWTManager != nil {
		victoriaLogsProxy, err := victorialogs.NewProxyHandler(s.VictoriaLogsURL)
		if err != nil {
			s.log.Error(err, "failed to create Victoria Logs proxy handler")
		} else {
			r.PathPrefix("/victorialogs/insert").Handler(
				http.StripPrefix("/victorialogs",
					s.JWTManager.Middleware(victoriaLogsProxy),
				),
			)
		}
	}

	// URLs that require authentication
	authenticated := r.NewRoute().Subrouter()
	authenticated.Use(a.RequireAuthenticationMiddleware)

	authenticated.Path("/user/avatar").HandlerFunc(auth.UserAvatar).Name("user:avatar")

	(&testruns.TestRunsController{Client: s.client, ControllerNamespace: s.ControllerNamespace}).ViewSet().RouteController(authenticated.PathPrefix("/test-runs").Subrouter())
	(&testscenarios.TestScenariosController{
		Client:                s.client,
		ControllerNamespace:   s.ControllerNamespace,
		DefaultScenarioCPU:    s.DefaultScenarioCPU,
		DefaultScenarioMemory: s.DefaultScenarioMemory,
	}).ViewSet().RouteController(authenticated.PathPrefix("/test-scenarios").Subrouter())

	(&workers.WorkersController{Client: s.client, ControllerNamespace: s.ControllerNamespace}).ViewSet().RouteController(authenticated.PathPrefix("/workers").Subrouter())
	(&influxdb.InfluxDBController{Client: s.client, Scheme: s.scheme, ControllerNamespace: s.ControllerNamespace}).ViewSet().RouteController(authenticated.PathPrefix("/influxdb").Subrouter())
	(&users.UsersController{Client: s.client, ControllerNamespace: s.ControllerNamespace, DexClient: s.DexClient}).ViewSet().RouteController(authenticated.PathPrefix("/settings/users").Subrouter())

	return &http.Server{
		Addr:         s.Addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}
