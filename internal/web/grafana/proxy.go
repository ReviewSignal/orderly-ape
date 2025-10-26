// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package grafana

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ReviewSignal/orderly-ape/internal/web/auth"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ProxyHandler handles proxying requests to Grafana
type ProxyHandler struct {
	grafanaURL string
	proxy      *httputil.ReverseProxy
}

// NewProxyHandler creates a new Grafana proxy handler
func NewProxyHandler(grafanaURL string) (*ProxyHandler, error) {
	target, err := url.Parse(grafanaURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the director to modify requests
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Set the X-WEBAUTH-USER header for Grafana auth
		req.Header.Set("X-WEBAUTH-USER", "orderly-ape")
		// Ensure the Host header is set correctly
		req.Host = target.Host
		// Set Origin header to avoid CORS issues
		req.Header.Set("Origin", target.Scheme+"://"+target.Host)
		// Set X-Forwarded headers
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Forwarded-Proto", target.Scheme)
		req.Header.Set("X-Forwarded-Port", "443")
		req.Header.Set("X-Forwarded-For", req.RemoteAddr)
	}

	// Add error handler for better logging
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger := log.FromContext(r.Context())
		logger.Error(err, "proxy error", "url", r.URL.String())
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("X-Frame-Options")
		resp.Header.Set("X-Frame-Options", "SAMEORIGIN")

		resp.Header.Del("Content-Security-Policy")
		resp.Header.Set("Content-Security-Policy", "frame-ancestors 'self';")
		return nil
	}

	return &ProxyHandler{
		grafanaURL: grafanaURL,
		proxy:      proxy,
	}, nil
}

// ServeHTTP handles the proxy request
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}

// DashboardHandler enforces kiosk mode and removes testid query parameter
func (h *ProxyHandler) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	// Ensure kiosk query parameter is set
	if !auth.UserIsAuthenticated(r.Context()) {
		query := r.URL.Query()
		query.Set("kiosk", "true")
		query.Del("var-testid")
		r.URL.RawQuery = query.Encode()
	}
	h.proxy.ServeHTTP(w, r)
}

// isAllowedPath checks if the request path is allowed for proxying
func isAllowedPath(path string) bool {
	allowedPrefixes := []string{
		"/grafana/d/",                          // Dashboard by UID
		"/grafana/api/frontend-metrics",        // Frontend metrics API
		"/grafana/api/annotations",             // Annotations API
		"/grafana/api/dashboards/",             // Dashboard API
		"/grafana/api/plugins/",                // Plugins API
		"/grafana/apis/dashboard.grafana.app/", // Dashboard app API
		"/grafana/api/datasources/proxy/",      // Datasource proxy for queries
		"/grafana/api/ds/query",                // Datasource query endpoint
		"/grafana/api/prometheus",              // Prometheus API
		"/grafana/public/",                     // Static assets
		"/grafana/avatar/",                     // User avatars
	}

	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// Middleware validates that only allowed paths are proxied for non-admin users
// Admin users can access all grafana paths
func (h *ProxyHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.UserIsAuthenticated(r.Context()) && !isAllowedPath(r.URL.Path) {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
