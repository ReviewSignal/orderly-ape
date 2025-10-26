// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package dex

import (
	"crypto/tls"

	dexapi "github.com/dexidp/dex/api/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps a DEX gRPC client for user management operations.
type Client struct {
	dexapi.DexClient

	conn *grpc.ClientConn
}

func NewInsecureClient(endpoint string) (*Client, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:      conn,
		DexClient: dexapi.NewDexClient(conn),
	}, nil
}

// NewClient creates a new DEX client connected to the specified endpoint.
// The endpoint should be in the format "host:port" (e.g., "dex.example.com:5557").
func NewClient(endpoint string) (*Client, error) {
	// Create TLS credentials with secure defaults
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// InsecureSkipVerify should be false in production
		// For production use, ensure proper certificate validation
	}
	creds := credentials.NewTLS(tlsConfig)

	// Establish connection
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:      conn,
		DexClient: dexapi.NewDexClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
