// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package info

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const gcloudMetadataHeaderKey = "Metadata-Flavor"
const gcloudMetadataHeaderValue = "Google"

// getGcloudGKEClusterName checks if the container is running on Google Kubernetes Engine (GKE).
func getGcloudGKEClusterName() string {
	if os.Getenv("GKE_CLUSTER_NAME") != "" {
		return os.Getenv("GKE_CLUSTER_NAME")
	}

	const gcloudMetadataURL = "http://metadata.google.internal/computeMetadata/v1/instance/attributes/cluster-name"
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	req, err := http.NewRequest("GET", gcloudMetadataURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Add(gcloudMetadataHeaderKey, gcloudMetadataHeaderValue)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			fmt.Println("Error closing response body:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	// If the metadata server returns a cluster name, we assume this is GKE
	return strings.TrimSpace(string(body))
}

// getCurrentGCPProject retrieves the current Google Cloud Project ID.
// It first checks common environment variables or config files, then queries the metadata server.
func getGcloudProjectID() string {
	// Check environment variables
	if project := os.Getenv("GOOGLE_CLOUD_PROJECT"); project != "" {
		return project
	}
	if project := os.Getenv("GCLOUD_PROJECT"); project != "" {
		return project
	}

	// Check metadata server
	const gcloudProjectMetadataURL = "http://metadata.google.internal/computeMetadata/v1/project/project-id"
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	req, err := http.NewRequest("GET", gcloudProjectMetadataURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Add(gcloudMetadataHeaderKey, gcloudMetadataHeaderValue)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			fmt.Println("Error closing response body:", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(body))
}

func init() {
	GCP = gcp{
		ProjectID:      getGcloudProjectID(),
		GKEClusterName: getGcloudGKEClusterName(),
	}
}
