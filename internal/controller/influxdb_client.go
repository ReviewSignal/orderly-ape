// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// InfluxDBClient is an interface for interacting with InfluxDB v2 API
type InfluxDBClient interface {
	// GetReady queries the /ready endpoint and returns version and server start time
	GetReady(ctx context.Context) (version string, startTime time.Time, err error)
	// ListBuckets lists all buckets for the organization
	ListBuckets(ctx context.Context) ([]string, error)
	// GetOrganizationID retrieves the organization ID by its name
	GetOrganizationID(ctx context.Context, org string) (string, error)
	// CreateToken creates a new token with write permissions to the specified bucket
	CreateToken(ctx context.Context, orgID, description, bucket string) (tokenID, tokenValue string, err error)
	// CreateReadOnlyToken creates a new token with read permissions to all buckets in the organization
	CreateReadOnlyToken(ctx context.Context, orgID, description string) (tokenID, tokenValue string, err error)
	// DeleteToken deletes a token by its ID
	DeleteToken(ctx context.Context, tokenID string) error
}

// influxDBHTTPClient implements InfluxDBClient using HTTP
type influxDBHTTPClient struct {
	address      string
	token        string
	organization string
	httpClient   *http.Client
}

// NewInfluxDBClient creates a new InfluxDB HTTP client
func NewInfluxDBClient(address, token, organization string) InfluxDBClient {
	return &influxDBHTTPClient{
		address:      address,
		token:        token,
		organization: organization,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetReady queries the /ready endpoint
func (c *influxDBHTTPClient) GetReady(ctx context.Context) (string, time.Time, error) {
	url := fmt.Sprintf("%s/ready", c.address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to query /ready: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Extract version from header
	version := resp.Header.Get("X-Influxdb-Version")

	// Parse response body for started time
	var readyResp struct {
		Started string `json:"started"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&readyResp); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode /ready response: %w", err)
	}

	// Parse started time
	startTime, err := time.Parse(time.RFC3339, readyResp.Started)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse started time: %w", err)
	}

	return version, startTime, nil
}

// GetOrganizationID retrieves the organization ID by its name
func (c *influxDBHTTPClient) GetOrganizationID(ctx context.Context, org string) (string, error) {
	url := fmt.Sprintf("%s/api/v2/orgs?org=%s", c.address, org)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to query organizations: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query organizations: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var orgsResp struct {
		Organizations []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"orgs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&orgsResp); err != nil {
		return "", fmt.Errorf("failed to decode buckets response: %w", err)
	}

	for _, o := range orgsResp.Organizations {
		if o.Name == org {
			return o.ID, nil
		}
	}

	return "", fmt.Errorf("organization %s not found", org)
}

// ListBuckets lists all buckets for the organization
func (c *influxDBHTTPClient) ListBuckets(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/api/v2/buckets?org=%s", c.address, c.organization)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query buckets: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var bucketsResp struct {
		Buckets []struct {
			Name string `json:"name"`
		} `json:"buckets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bucketsResp); err != nil {
		return nil, fmt.Errorf("failed to decode buckets response: %w", err)
	}

	buckets := make([]string, 0, len(bucketsResp.Buckets))
	for _, b := range bucketsResp.Buckets {
		buckets = append(buckets, b.Name)
	}

	return buckets, nil
}

// CreateToken creates a new token with write permissions to the specified bucket
func (c *influxDBHTTPClient) CreateToken(ctx context.Context, orgID, description, bucket string) (string, string, error) {
	url := fmt.Sprintf("%s/api/v2/authorizations", c.address)

	// Prepare the request body
	requestBody := map[string]interface{}{
		"description": description,
		"orgID":       orgID,
		"permissions": []map[string]interface{}{
			{
				"action": "write",
				"resource": map[string]interface{}{
					"type":  "buckets",
					"name":  bucket,
					"orgID": orgID,
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, io.NopCloser(bytes.NewReader(bodyBytes)))
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to create token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.ID, tokenResp.Token, nil
}

// CreateReadOnlyToken creates a new token with read permissions to all buckets in the organization
func (c *influxDBHTTPClient) CreateReadOnlyToken(ctx context.Context, orgID, description string) (string, string, error) {
	url := fmt.Sprintf("%s/api/v2/authorizations", c.address)

	// Prepare the request body with read permission to all buckets in the organization
	requestBody := map[string]interface{}{
		"description": description,
		"orgID":       orgID,
		"permissions": []map[string]interface{}{
			{
				"action": "read",
				"resource": map[string]interface{}{
					"type":  "buckets",
					"orgID": orgID,
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, io.NopCloser(bytes.NewReader(bodyBytes)))
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to create token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.ID, tokenResp.Token, nil
}

// DeleteToken deletes a token by its ID
func (c *influxDBHTTPClient) DeleteToken(ctx context.Context, tokenID string) error {
	url := fmt.Sprintf("%s/api/v2/authorizations/%s", c.address, tokenID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Accept both 204 No Content (successful deletion) and 404 Not Found (already deleted)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
