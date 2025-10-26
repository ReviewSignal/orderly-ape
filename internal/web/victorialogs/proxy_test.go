// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package victorialogs_test

import (
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ReviewSignal/orderly-ape/internal/web/victorialogs"
)

var _ = Describe("ProxyHandler", func() {
	var (
		backend        *httptest.Server
		proxyHandler   *victorialogs.ProxyHandler
		backendCalled  bool
		backendHeaders http.Header
		backendPath    string
	)

	BeforeEach(func() {
		backendCalled = false
		backendHeaders = nil
		backendPath = ""

		// Create a test backend server
		backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			backendCalled = true
			backendHeaders = r.Header.Clone()
			backendPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))

		var err error
		proxyHandler, err = victorialogs.NewProxyHandler(backend.URL)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if backend != nil {
			backend.Close()
		}
	})

	Describe("NewProxyHandler", func() {
		It("should create a proxy handler with valid URL", func() {
			handler, err := victorialogs.NewProxyHandler("http://localhost:9428")
			Expect(err).NotTo(HaveOccurred())
			Expect(handler).NotTo(BeNil())
		})

		It("should fail with invalid URL", func() {
			handler, err := victorialogs.NewProxyHandler("://invalid")
			Expect(err).To(HaveOccurred())
			Expect(handler).To(BeNil())
		})
	})

	Describe("ServeHTTP", func() {
		It("should proxy requests to backend", func() {
			req := httptest.NewRequest("POST", "/insert/jsonline", nil)
			w := httptest.NewRecorder()

			proxyHandler.ServeHTTP(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should preserve request path", func() {
			req := httptest.NewRequest("POST", "/insert/jsonline", nil)
			w := httptest.NewRecorder()

			proxyHandler.ServeHTTP(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(backendPath).To(Equal("/insert/jsonline"))
		})

		It("should set X-Forwarded headers", func() {
			req := httptest.NewRequest("POST", "/insert/jsonline", nil)
			w := httptest.NewRecorder()

			proxyHandler.ServeHTTP(w, req)

			Expect(backendCalled).To(BeTrue())
			Expect(backendHeaders.Get("X-Forwarded-Host")).NotTo(BeEmpty())
			Expect(backendHeaders.Get("X-Forwarded-Proto")).NotTo(BeEmpty())
			Expect(backendHeaders.Get("X-Forwarded-For")).NotTo(BeEmpty())
		})
	})

	Describe("Error handling", func() {
		It("should handle backend errors gracefully", func() {
			// Create a handler with an invalid backend URL
			invalidHandler, err := victorialogs.NewProxyHandler("http://localhost:1")
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("POST", "/insert/jsonline", nil)
			w := httptest.NewRecorder()

			invalidHandler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadGateway))
			bodyBytes, _ := io.ReadAll(w.Body)
			Expect(string(bodyBytes)).To(ContainSubstring("Bad Gateway"))
		})
	})
})
