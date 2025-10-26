// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package jwt

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Handler provides HTTP endpoints for JWT operations
type Handler struct {
	manager *Manager
}

// NewHandler creates a new JWT handler
func NewHandler(manager *Manager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// HandleJWKS returns the public key in JWKS format
// GET /.well-known/jwks.json
func (h *Handler) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	logger := log.FromContext(r.Context())

	jwks, err := h.manager.GetPublicKeyJWKS()
	if err != nil {
		logger.Error(err, "failed to generate JWKS")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jwks)
}

// HandleVerify verifies a JWT token from the Authorization header
// GET /jwt/verify?aud=foo.bar
func (h *Handler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	logger := log.FromContext(r.Context())

	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		h.sendError(w, logger, http.StatusUnauthorized, "missing authorization header", nil)
		return
	}

	// Extract bearer token
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		h.sendError(w, logger, http.StatusUnauthorized, "invalid authorization header format", nil)
		return
	}

	token := parts[1]

	// Verify the token
	claims, err := h.manager.VerifyToken(token)
	if err != nil {
		h.sendError(w, logger, http.StatusUnauthorized, "invalid token", err)
		return
	}

	// Validate query parameter claims
	query := r.URL.Query()
	for key, values := range query {
		if len(values) > 0 {
			expectedValue := values[0]
			if err := ValidateClaim(claims, key, expectedValue); err != nil {
				h.sendError(w, logger, http.StatusForbidden, "claim validation failed", err)
				return
			}
		}
	}

	// Token is valid
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]any{
		"valid":   true,
		"subject": claims["sub"],
	}
	_ = json.NewEncoder(w).Encode(response)
}

// sendError writes an error response
func (h *Handler) sendError(w http.ResponseWriter, logger logr.Logger, statusCode int, message string, err error) {
	if err != nil {
		logger.Error(err, message)
	} else {
		logger.Info(message)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := map[string]string{
		"error": message,
	}
	_ = json.NewEncoder(w).Encode(response)
}
