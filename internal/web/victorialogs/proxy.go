// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package victorialogs

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ProxyHandler handles proxying requests to Victoria Logs
type ProxyHandler struct {
	victoriaLogsURL string
	proxy           *httputil.ReverseProxy
}

// NewProxyHandler creates a new Victoria Logs proxy handler
func NewProxyHandler(victoriaLogsURL string) (*ProxyHandler, error) {
	target, err := url.Parse(victoriaLogsURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the director to modify requests
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Ensure the Host header is set correctly
		req.Host = target.Host
		// Set X-Forwarded headers
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Forwarded-Proto", target.Scheme)
		req.Header.Set("X-Forwarded-For", req.RemoteAddr)
	}

	// Add error handler for better logging
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger := log.FromContext(r.Context())
		logger.Error(err, "proxy error", "url", r.URL.String())
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return &ProxyHandler{
		victoriaLogsURL: victoriaLogsURL,
		proxy:           proxy,
	}, nil
}

// ServeHTTP handles the proxy request
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}
