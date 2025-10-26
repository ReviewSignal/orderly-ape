// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package jwt_test

import (
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

var _ = Describe("JWT Manager", func() {
	var (
		manager        *jwt.Manager
		privateKeyPath string
		publicKeyPath  string
		testDir        string
	)

	BeforeEach(func() {
		// Create a temporary directory for test keys
		var err error
		testDir, err = os.MkdirTemp("", "jwt-test-*")
		Expect(err).NotTo(HaveOccurred())

		privateKeyPath = filepath.Join(testDir, "private.pem")
		publicKeyPath = filepath.Join(testDir, "public.pem")

		// Generate RSA key pair for testing
		generateTestKeys(privateKeyPath, publicKeyPath)

		config := jwt.Config{
			PrivateKeyPath: privateKeyPath,
			PublicKeyPath:  publicKeyPath,
			Issuer:         "https://test.example.com",
		}

		manager, err = jwt.NewManager(config)
		Expect(err).NotTo(HaveOccurred())
		Expect(manager).NotTo(BeNil())
	})

	AfterEach(func() {
		if testDir != "" {
			_ = os.RemoveAll(testDir)
		}
	})

	Describe("NewManager", func() {
		It("should create a manager with valid keys", func() {
			config := jwt.Config{
				PrivateKeyPath: privateKeyPath,
				PublicKeyPath:  publicKeyPath,
				Issuer:         "https://test.example.com",
			}

			m, err := jwt.NewManager(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
		})

		It("should fail with non-existent private key", func() {
			config := jwt.Config{
				PrivateKeyPath: "/nonexistent/private.pem",
				PublicKeyPath:  publicKeyPath,
				Issuer:         "https://test.example.com",
			}

			_, err := jwt.NewManager(config)
			Expect(err).To(HaveOccurred())
		})

		It("should fail with non-existent public key", func() {
			config := jwt.Config{
				PrivateKeyPath: privateKeyPath,
				PublicKeyPath:  "/nonexistent/public.pem",
				Issuer:         "https://test.example.com",
			}

			_, err := jwt.NewManager(config)
			Expect(err).To(HaveOccurred())
		})

		It("should derive public key from private key when public key path is not provided", func() {
			config := jwt.Config{
				PrivateKeyPath: privateKeyPath,
				PublicKeyPath:  "", // Empty public key path
				Issuer:         "https://test.example.com",
			}

			m, err := jwt.NewManager(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())

			// Verify we can generate and verify tokens
			token, err := m.GenerateToken("testuser", 1*time.Hour, "aud", "test-app")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			// Verify the token
			claims, err := m.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())
			Expect(claims["sub"]).To(Equal("testuser"))
			Expect(claims["aud"]).To(Equal("test-app"))
		})
	})

	Describe("GenerateToken", func() {
		It("should generate a valid token with basic claims", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour)
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
		})

		It("should generate a token with custom claims", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour, "aud", "my-app", "email", "user@example.com", "role", "admin")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
		})

		It("should generate tokens with different expiry times", func() {
			token1, err := manager.GenerateToken("user123", 1*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			token2, err := manager.GenerateToken("user123", 24*time.Hour)
			Expect(err).NotTo(HaveOccurred())

			Expect(token1).NotTo(Equal(token2))
		})
	})

	Describe("VerifyToken", func() {
		It("should verify a valid token", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour)
			Expect(err).NotTo(HaveOccurred())

			claims, err := manager.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())
			Expect(claims).NotTo(BeNil())
			Expect(claims["sub"]).To(Equal("user123"))
			Expect(claims["iss"]).To(Equal("https://test.example.com"))
		})

		It("should verify a token with custom claims", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour, "aud", "my-app", "email", "user@example.com")
			Expect(err).NotTo(HaveOccurred())

			claims, err := manager.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())
			Expect(claims["sub"]).To(Equal("user123"))
			Expect(claims["aud"]).To(Equal("my-app"))
			Expect(claims["email"]).To(Equal("user@example.com"))
		})

		It("should reject an invalid token", func() {
			_, err := manager.VerifyToken("invalid.token.here")
			Expect(err).To(HaveOccurred())
		})

		It("should reject a token with wrong issuer", func() {
			// Create a manager with different issuer
			config := jwt.Config{
				PrivateKeyPath: privateKeyPath,
				PublicKeyPath:  publicKeyPath,
				Issuer:         "https://different.example.com",
			}
			differentManager, err := jwt.NewManager(config)
			Expect(err).NotTo(HaveOccurred())

			token, err := differentManager.GenerateToken("user123", 1*time.Hour)
			Expect(err).NotTo(HaveOccurred())

			// Try to verify with original manager
			_, err = manager.VerifyToken(token)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid issuer"))
		})

		It("should reject an expired token", func() {
			token, err := manager.GenerateToken("user123", -1*time.Hour)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.VerifyToken(token)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ValidateClaim", func() {
		It("should validate a matching string claim", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour, "aud", "my-app")
			Expect(err).NotTo(HaveOccurred())

			claims, err := manager.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())

			err = jwt.ValidateClaim(claims, "aud", "my-app")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation for mismatched claim", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour, "aud", "my-app")
			Expect(err).NotTo(HaveOccurred())

			claims, err := manager.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())

			err = jwt.ValidateClaim(claims, "aud", "different-app")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mismatch"))
		})

		It("should fail validation for missing claim", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour)
			Expect(err).NotTo(HaveOccurred())

			claims, err := manager.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())

			err = jwt.ValidateClaim(claims, "aud", "my-app")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should validate an array claim", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour, "aud", []string{"app1", "app2", "app3"})
			Expect(err).NotTo(HaveOccurred())

			claims, err := manager.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())

			err = jwt.ValidateClaim(claims, "aud", "app2")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation for array claim without match", func() {
			token, err := manager.GenerateToken("user123", 1*time.Hour, "aud", []string{"app1", "app2", "app3"})
			Expect(err).NotTo(HaveOccurred())

			claims, err := manager.VerifyToken(token)
			Expect(err).NotTo(HaveOccurred())

			err = jwt.ValidateClaim(claims, "aud", "app4")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not contain"))
		})
	})

	Describe("GetPublicKeyJWKS", func() {
		It("should return JWKS format", func() {
			jwks, err := manager.GetPublicKeyJWKS()
			Expect(err).NotTo(HaveOccurred())
			Expect(jwks).NotTo(BeEmpty())

			// Verify it's valid JSON
			Expect(jwks[0]).To(Equal(byte('{')))
		})

		It("should contain required JWKS fields", func() {
			jwks, err := manager.GetPublicKeyJWKS()
			Expect(err).NotTo(HaveOccurred())

			jwksStr := string(jwks)
			Expect(jwksStr).To(ContainSubstring("\"keys\""))
			Expect(jwksStr).To(ContainSubstring("\"kty\""))
			Expect(jwksStr).To(ContainSubstring("\"RSA\""))
			Expect(jwksStr).To(ContainSubstring("\"use\""))
			Expect(jwksStr).To(ContainSubstring("\"sig\""))
			Expect(jwksStr).To(ContainSubstring("\"alg\""))
			Expect(jwksStr).To(ContainSubstring("\"RS256\""))
		})
	})

	Describe("Middleware", func() {
		var (
			handler http.Handler
			token   string
		)

		BeforeEach(func() {
			// Generate a valid token
			var err error
			token, err = manager.GenerateToken("test-user", 1*time.Hour)
			Expect(err).NotTo(HaveOccurred())

			// Create a simple handler that returns 200 OK
			handler = manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			}))
		})

		It("should allow requests with valid JWT token", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(Equal("success"))
		})

		It("should reject requests without Authorization header", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			Expect(w.Body.String()).To(ContainSubstring("missing authorization header"))
		})

		It("should reject requests with invalid Authorization header format", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "InvalidFormat "+token)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			Expect(w.Body.String()).To(ContainSubstring("invalid authorization header format"))
		})

		It("should reject requests with malformed token", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer invalid.token.here")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			Expect(w.Body.String()).To(ContainSubstring("invalid token"))
		})

		It("should reject requests with expired token", func() {
			// Generate an expired token
			expiredToken, err := manager.GenerateToken("test-user", -1*time.Hour)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+expiredToken)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			Expect(w.Body.String()).To(ContainSubstring("invalid token"))
		})

		It("should handle case-insensitive Bearer scheme", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "bearer "+token)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(Equal("success"))
		})
	})
})

// Helper function to generate test RSA keys
func generateTestKeys(privateKeyPath, publicKeyPath string) {
	// Use openssl to generate keys
	cmd := "openssl genrsa -out " + privateKeyPath + " 2048 2>/dev/null && openssl rsa -in " + privateKeyPath + " -pubout -out " + publicKeyPath + " 2>/dev/null"
	err := exec.Command("sh", "-c", cmd).Run()
	Expect(err).NotTo(HaveOccurred())
}
