// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package jwt_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ReviewSignal/orderly-ape/internal/jwt"
)

var _ = Describe("JWT Handler", func() {
	var (
		manager        *jwt.Manager
		handler        *jwt.Handler
		privateKeyPath string
		publicKeyPath  string
		testDir        string
	)

	BeforeEach(func() {
		var err error
		testDir, err = os.MkdirTemp("", "jwt-handler-test-*")
		Expect(err).NotTo(HaveOccurred())

		privateKeyPath = filepath.Join(testDir, "private.pem")
		publicKeyPath = filepath.Join(testDir, "public.pem")

		generateTestKeysForHandler(privateKeyPath, publicKeyPath)

		config := jwt.Config{
			PrivateKeyPath: privateKeyPath,
			PublicKeyPath:  publicKeyPath,
			Issuer:         "https://test.example.com",
		}

		manager, err = jwt.NewManager(config)
		Expect(err).NotTo(HaveOccurred())

		handler = jwt.NewHandler(manager)
	})

	AfterEach(func() {
		if testDir != "" {
			_ = os.RemoveAll(testDir)
		}
	})

	Describe("HandleJWKS", func() {
		It("should return JWKS JSON", func() {
			req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
			w := httptest.NewRecorder()

			handler.HandleJWKS(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var jwks map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &jwks)
			Expect(err).NotTo(HaveOccurred())

			Expect(jwks).To(HaveKey("keys"))
			keys, ok := jwks["keys"].([]any)
			Expect(ok).To(BeTrue())
			Expect(keys).NotTo(BeEmpty())
		})
	})

	Describe("HandleVerify", func() {
		var validToken string

		BeforeEach(func() {
			var err error
			validToken, err = manager.GenerateToken("testuser", 1*time.Hour, "aud", "test-app")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should verify a valid token", func() {
			req := httptest.NewRequest(http.MethodGet, "/jwt/verify", nil)
			req.Header.Set("Authorization", "Bearer "+validToken)
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response["valid"]).To(BeTrue())
			Expect(response["subject"]).To(Equal("testuser"))
		})

		It("should verify token with matching audience claim", func() {
			req := httptest.NewRequest(http.MethodGet, "/jwt/verify?aud=test-app", nil)
			req.Header.Set("Authorization", "Bearer "+validToken)
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should reject token with mismatched audience claim", func() {
			req := httptest.NewRequest(http.MethodGet, "/jwt/verify?aud=wrong-app", nil)
			req.Header.Set("Authorization", "Bearer "+validToken)
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusForbidden))

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response["error"]).To(ContainSubstring("claim validation failed"))
		})

		It("should reject request without authorization header", func() {
			req := httptest.NewRequest(http.MethodGet, "/jwt/verify", nil)
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response["error"]).To(ContainSubstring("missing authorization header"))
		})

		It("should reject request with invalid authorization format", func() {
			req := httptest.NewRequest(http.MethodGet, "/jwt/verify", nil)
			req.Header.Set("Authorization", "InvalidFormat")
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response["error"]).To(ContainSubstring("invalid authorization header format"))
		})

		It("should reject invalid token", func() {
			req := httptest.NewRequest(http.MethodGet, "/jwt/verify", nil)
			req.Header.Set("Authorization", "Bearer invalid.token.here")
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response["error"]).To(ContainSubstring("invalid token"))
		})

		It("should verify token with multiple query parameter validations", func() {
			tokenWithClaims, err := manager.GenerateToken("testuser", 1*time.Hour, "aud", "test-app", "scope", "read:data")
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/jwt/verify?aud=test-app&scope=read:data", nil)
			req.Header.Set("Authorization", "Bearer "+tokenWithClaims)
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should reject if one of multiple claims doesn't match", func() {
			tokenWithClaims, err := manager.GenerateToken("testuser", 1*time.Hour, "aud", "test-app", "scope", "read:data")
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodGet, "/jwt/verify?aud=test-app&scope=write:data", nil)
			req.Header.Set("Authorization", "Bearer "+tokenWithClaims)
			w := httptest.NewRecorder()

			handler.HandleVerify(w, req)

			Expect(w.Code).To(Equal(http.StatusForbidden))
		})
	})
})

func generateTestKeysForHandler(privateKeyPath, publicKeyPath string) {
	cmd := "openssl genrsa -out " + privateKeyPath + " 2048 2>/dev/null && openssl rsa -in " + privateKeyPath + " -pubout -out " + publicKeyPath + " 2>/dev/null"
	err := exec.Command("sh", "-c", cmd).Run()
	Expect(err).NotTo(HaveOccurred())
}
