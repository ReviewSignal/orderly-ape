// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

// Package jwt provides JWT token generation, verification, and HTTP handlers.
//
// Example usage:
//
//	// Initialize JWT manager with both keys
//	config := jwt.Config{
//		PrivateKeyPath: "/path/to/private.pem",
//		PublicKeyPath:  "/path/to/public.pem",
//		Issuer:         "https://your-issuer.com",
//	}
//	manager, err := jwt.NewManager(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Or initialize with only private key (public key will be derived)
//	config := jwt.Config{
//		PrivateKeyPath: "/path/to/private.pem",
//		PublicKeyPath:  "", // Empty - public key will be derived
//		Issuer:         "https://your-issuer.com",
//	}
//	manager, err := jwt.NewManager(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Generate a token
//	token, err := manager.GenerateToken("user123", map[string]interface{}{
//		"aud":   "my-app",
//		"email": "user@example.com",
//	}, 1*time.Hour)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Verify a token
//	claims, err := manager.VerifyToken(token)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Validate specific claims
//	if err := jwt.ValidateClaim(claims, "aud", "my-app"); err != nil {
//		log.Fatal(err)
//	}
package jwt
