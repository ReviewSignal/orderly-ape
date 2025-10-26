// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package grafana_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ReviewSignal/orderly-ape/internal/web/grafana"
)

func TestGrafanaProxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grafana Proxy Suite")
}

var _ = Describe("ProxyHandler", func() {
	var (
		backend        *httptest.Server
		proxyHandler   *grafana.ProxyHandler
		backendCalled  bool
		backendHeaders http.Header
	)

	BeforeEach(func() {
		backendCalled = false
		backendHeaders = nil

		// Create a test backend server
		backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			backendCalled = true
			backendHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))

		var err error
		proxyHandler, err = grafana.NewProxyHandler(backend.URL)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if backend != nil {
			backend.Close()
		}
	})

	Describe("NewProxyHandler", func() {
		It("should create a proxy handler with valid URL", func() {
			handler, err := grafana.NewProxyHandler("http://localhost:3000")
			Expect(err).NotTo(HaveOccurred())
			Expect(handler).NotTo(BeNil())
		})

		It("should fail with invalid URL", func() {
			handler, err := grafana.NewProxyHandler("://invalid")
			Expect(err).To(HaveOccurred())
			Expect(handler).To(BeNil())
		})
	})

	Describe("ServeHTTP", func() {
		It("should proxy requests to backend", func() {
			req := httptest.NewRequest("GET", "/api/dashboards/uid/test", nil)
			w := httptest.NewRecorder()

			proxyHandler.ServeHTTP(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should set X-WEBAUTH-USER header", func() {
			req := httptest.NewRequest("GET", "/api/dashboards/uid/test", nil)
			w := httptest.NewRecorder()

			proxyHandler.ServeHTTP(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(backendHeaders.Get("X-Webauth-User")).To(Equal("orderly-ape"))
		})
	})

	Describe("DashboardHandler", func() {
		It("should add kiosk parameter if not present", func() {
			req := httptest.NewRequest("GET", "/d/abc123/test-dashboard", nil)
			w := httptest.NewRecorder()

			proxyHandler.DashboardHandler(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(req.URL.Query().Get("kiosk")).To(Equal("true"))
		})

		It("should enforce kiosk parameter", func() {
			req := httptest.NewRequest("GET", "/d/abc123/test-dashboard?kiosk=full", nil)
			w := httptest.NewRecorder()

			proxyHandler.DashboardHandler(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(req.URL.Query().Get("kiosk")).To(Equal("true"))
		})

		It("should preserve other query parameters", func() {
			req := httptest.NewRequest("GET", "/d/abc123/test?var-host=server1&from=now-1h", nil)
			w := httptest.NewRecorder()

			proxyHandler.DashboardHandler(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(req.URL.Query().Get("var-host")).To(Equal("server1"))
			Expect(req.URL.Query().Get("from")).To(Equal("now-1h"))
			Expect(req.URL.Query().Get("kiosk")).To(Equal("true"))
		})
	})

	Describe("Middleware", func() {
		It("should allow dashboard URLs", func() {
			allowedPaths := []string{
				"/grafana/d/abc123/test-dashboard",
				"/grafana/api/dashboards/uid/abc123",
				"/grafana/api/datasources/proxy/1/query",
				"/grafana/api/ds/query",
				"/grafana/public/img/grafana_icon.svg",
				"/grafana/avatar/abc123",
			}

			for _, path := range allowedPaths {
				backendCalled = false
				req := httptest.NewRequest("GET", path, nil)
				w := httptest.NewRecorder()

				handler := proxyHandler.Middleware(proxyHandler)
				handler.ServeHTTP(w, req)

				Expect(backendCalled).To(BeTrue(), "Expected %s to be allowed", path)
			}
		})

		It("should block non-dashboard URLs", func() {
			blockedPaths := []string{
				"/api/admin/users",
				"/api/org",
				"/api/user/preferences",
				"/api/folders",
				"/login",
				"/logout",
			}

			for _, path := range blockedPaths {
				backendCalled = false
				req := httptest.NewRequest("GET", path, nil)
				w := httptest.NewRecorder()

				handler := proxyHandler.Middleware(proxyHandler)
				handler.ServeHTTP(w, req)

				Expect(backendCalled).To(BeFalse(), "Expected %s to be blocked", path)
				Expect(w.Code).To(Equal(http.StatusNotFound))
			}
		})
	})

	Describe("Error handling", func() {
		It("should handle backend errors gracefully", func() {
			// Create a handler with an invalid backend URL
			invalidHandler, err := grafana.NewProxyHandler("http://localhost:1")
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/dashboards/uid/test", nil)
			w := httptest.NewRecorder()

			invalidHandler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadGateway))
			bodyBytes, _ := io.ReadAll(w.Body)
			Expect(string(bodyBytes)).To(ContainSubstring("Bad Gateway"))
		})
	})
})
