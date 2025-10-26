// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrNotFound is returned when a Grafana resource is not found
var ErrNotFound = errors.New("resource not found")

// GrafanaClient is an interface for interacting with Grafana API
type GrafanaClient interface {
	// CreateDatasource creates a new datasource in Grafana
	CreateDatasource(ctx context.Context, ds *GrafanaDatasource) (uid string, err error)
	// UpdateDatasource updates an existing datasource in Grafana by UID
	UpdateDatasource(ctx context.Context, uid string, ds *GrafanaDatasource) error
	// GetDatasource retrieves a datasource by UID
	GetDatasource(ctx context.Context, uid string) (*GrafanaDatasource, error)
	// DeleteDatasource deletes a datasource by UID
	DeleteDatasource(ctx context.Context, uid string) error
	// CreateDashboard creates a new dashboard in Grafana
	CreateDashboard(ctx context.Context, dashboard map[string]any) (uid string, err error)
	// UpdateDashboard updates an existing dashboard in Grafana by UID
	UpdateDashboard(ctx context.Context, uid string, dashboard map[string]any) error
	// GetDashboard retrieves a dashboard by UID
	GetDashboard(ctx context.Context, uid string) (map[string]any, error)
	// DeleteDashboard deletes a dashboard by UID
	DeleteDashboard(ctx context.Context, uid string) error
}

// GrafanaDatasource represents a Grafana datasource configuration
type GrafanaDatasource struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TypeName string `json:"typeName,omitempty"`
	URL      string `json:"url"`
	Access   string `json:"access"`
	OrgID    int    `json:"orgId,omitempty"`
	// UID is used for updates and responses
	UID            string            `json:"uid,omitempty"`
	IsDefault      bool              `json:"isDefault"`
	JSONData       map[string]any    `json:"jsonData"`
	SecureJSONData map[string]string `json:"secureJsonData,omitempty"`
}

// grafanaHTTPClient implements GrafanaClient using HTTP
type grafanaHTTPClient struct {
	apiURL     string
	token      string
	httpClient *http.Client
}

// NewGrafanaClient creates a new Grafana HTTP client
func NewGrafanaClient(apiURL, token string) GrafanaClient {
	return &grafanaHTTPClient{
		apiURL: apiURL,
		token:  token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CreateDatasource creates a new datasource in Grafana
func (c *grafanaHTTPClient) CreateDatasource(ctx context.Context, ds *GrafanaDatasource) (string, error) {
	url := fmt.Sprintf("%s/api/datasources", c.apiURL)

	bodyBytes, err := json.Marshal(ds)
	if err != nil {
		return "", fmt.Errorf("failed to marshal datasource: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create datasource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Datasource struct {
			UID string `json:"uid"`
		} `json:"datasource"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Datasource.UID, nil
}

// UpdateDatasource updates an existing datasource in Grafana by UID
func (c *grafanaHTTPClient) UpdateDatasource(ctx context.Context, uid string, ds *GrafanaDatasource) error {
	url := fmt.Sprintf("%s/api/datasources/uid/%s", c.apiURL, uid)

	// Ensure UID is set in the datasource
	ds.UID = uid

	bodyBytes, err := json.Marshal(ds)
	if err != nil {
		return fmt.Errorf("failed to marshal datasource: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update datasource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetDatasource retrieves a datasource by UID
func (c *grafanaHTTPClient) GetDatasource(ctx context.Context, uid string) (*GrafanaDatasource, error) {
	url := fmt.Sprintf("%s/api/datasources/uid/%s", c.apiURL, uid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var ds GrafanaDatasource
	if err := json.NewDecoder(resp.Body).Decode(&ds); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ds, nil
}

// DeleteDatasource deletes a datasource by UID
func (c *grafanaHTTPClient) DeleteDatasource(ctx context.Context, uid string) error {
	url := fmt.Sprintf("%s/api/datasources/uid/%s", c.apiURL, uid)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete datasource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Accept both 200 OK (successful deletion) and 404 Not Found (already deleted)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateDashboard creates a new dashboard in Grafana
func (c *grafanaHTTPClient) CreateDashboard(ctx context.Context, dashboard map[string]any) (string, error) {
	url := fmt.Sprintf("%s/api/dashboards/db", c.apiURL)

	payload := map[string]any{
		"dashboard": dashboard,
		"overwrite": false,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal dashboard: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create dashboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		UID string `json:"uid"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.UID, nil
}

// UpdateDashboard updates an existing dashboard in Grafana by UID
func (c *grafanaHTTPClient) UpdateDashboard(ctx context.Context, uid string, dashboard map[string]any) error {
	url := fmt.Sprintf("%s/api/dashboards/db", c.apiURL)

	// Ensure UID is set in the dashboard
	dashboard["uid"] = uid

	payload := map[string]any{
		"dashboard": dashboard,
		"overwrite": true,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal dashboard: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update dashboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetDashboard retrieves a dashboard by UID
func (c *grafanaHTTPClient) GetDashboard(ctx context.Context, uid string) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/dashboards/uid/%s", c.apiURL, uid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// DeleteDashboard deletes a dashboard by UID
func (c *grafanaHTTPClient) DeleteDashboard(ctx context.Context, uid string) error {
	url := fmt.Sprintf("%s/api/dashboards/uid/%s", c.apiURL, uid)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete dashboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Accept both 200 OK (successful deletion) and 404 Not Found (already deleted)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
