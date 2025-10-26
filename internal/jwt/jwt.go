// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Manager handles JWT token generation and verification
type Manager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
}

// Config holds JWT configuration
type Config struct {
	PrivateKeyPath string
	PublicKeyPath  string
	Issuer         string
}

// NewManager creates a new JWT manager
func NewManager(config Config) (*Manager, error) {
	privateKey, err := loadPrivateKey(config.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	var publicKey *rsa.PublicKey
	if config.PublicKeyPath != "" {
		// Load public key from file if provided
		publicKey, err = loadPublicKey(config.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load public key: %w", err)
		}
	} else {
		// Derive public key from private key if not provided
		publicKey = &privateKey.PublicKey
	}

	return &Manager{
		privateKey: privateKey,
		publicKey:  publicKey,
		issuer:     config.Issuer,
	}, nil
}

// GenerateToken creates a new JWT with custom claims
func (m *Manager) GenerateToken(subject string, expiry time.Duration, claims ...any) (string, error) {
	if len(claims)%2 != 0 {
		return "", fmt.Errorf("claims must be key-value pairs")
	}

	now := time.Now()

	jwtClaims := jwt.MapClaims{
		"iss": m.issuer,
		"sub": subject,
		"iat": now.Unix(),
		"exp": now.Add(expiry).Unix(),
	}

	// Add custom claims
	for i := 0; i < len(claims); i += 2 {
		k, ok := claims[i].(string)
		if !ok {
			return "", fmt.Errorf("claim keys must be strings")
		}
		jwtClaims[k] = claims[i+1]
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtClaims)

	return token.SignedString(m.privateKey)
}

// VerifyToken validates a JWT and returns the claims
func (m *Manager) VerifyToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		// Verify the signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Verify issuer
	if iss, ok := claims["iss"].(string); !ok || iss != m.issuer {
		return nil, fmt.Errorf("invalid issuer")
	}

	return claims, nil
}

// ValidateClaim checks if a specific claim matches the expected value
func ValidateClaim(claims jwt.MapClaims, key string, expectedValue string) error {
	value, ok := claims[key]
	if !ok {
		return fmt.Errorf("claim %s not found", key)
	}

	// Handle both string and []string for audience
	switch v := value.(type) {
	case string:
		if v != expectedValue {
			return fmt.Errorf("claim %s mismatch: got %s, expected %s", key, v, expectedValue)
		}
	case []any:
		found := false
		for _, item := range v {
			if str, ok := item.(string); ok && str == expectedValue {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("claim %s does not contain expected value %s", key, expectedValue)
		}
	default:
		return fmt.Errorf("claim %s has unsupported type", key)
	}

	return nil
}

// GetPublicKeyJWKS returns the public key in JWKS format
func (m *Manager) GetPublicKeyJWKS() ([]byte, error) {
	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"n":   encodeBase64URL(m.publicKey.N.Bytes()),
				"e":   encodeBase64URL(bigIntToBytes(m.publicKey.E)),
			},
		},
	}

	return json.Marshal(jwks)
}

// Helper functions

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 format
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}

	return rsaKey, nil
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Try parsing as certificate
		cert, certErr := x509.ParseCertificate(block.Bytes)
		if certErr != nil {
			return nil, err
		}
		key = cert.PublicKey
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaKey, nil
}

func encodeBase64URL(data []byte) string {
	// JWT uses base64url encoding without padding
	const base64URL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	encoded := make([]byte, (len(data)*8+5)/6)

	bits := uint32(0)
	bitsLen := uint(0)
	pos := 0

	for _, b := range data {
		bits = (bits << 8) | uint32(b)
		bitsLen += 8

		for bitsLen >= 6 {
			bitsLen -= 6
			encoded[pos] = base64URL[(bits>>bitsLen)&0x3F]
			pos++
		}
	}

	if bitsLen > 0 {
		bits <<= (6 - bitsLen)
		encoded[pos] = base64URL[bits&0x3F]
		pos++
	}

	return string(encoded[:pos])
}

func bigIntToBytes(n int) []byte {
	// Convert int to bytes in big-endian format
	bytes := make([]byte, 4)
	bytes[0] = byte(n >> 24)
	bytes[1] = byte(n >> 16)
	bytes[2] = byte(n >> 8)
	bytes[3] = byte(n)

	// Trim leading zeros
	start := 0
	for start < len(bytes)-1 && bytes[start] == 0 {
		start++
	}

	return bytes[start:]
}

// Middleware returns an HTTP middleware that validates JWT tokens
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			sendJSONError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		// Extract bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			sendJSONError(w, http.StatusUnauthorized, "invalid authorization header format")
			return
		}

		token := parts[1]

		// Verify the token
		_, err := m.VerifyToken(token)
		if err != nil {
			sendJSONError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		// Token is valid, proceed to next handler
		next.ServeHTTP(w, r)
	})
}

// sendJSONError sends a JSON error response
func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := map[string]string{
		"error": message,
	}
	_ = json.NewEncoder(w).Encode(response)
}
