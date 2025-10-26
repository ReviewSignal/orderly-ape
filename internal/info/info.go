// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package info

import (
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// Info represents metadata about current running instance
type version struct {
	BuildDate    string `json:"build_date"`
	Environment  string `json:"environment"`
	GitCommit    string `json:"git_commit"`
	GitTreeState string `json:"git_tree_state"`
	GitVersion   string `json:"git_version"`
}

var Version version

type gcp struct {
	ProjectID      string `json:"project_id"`
	Region         string `json:"region"`
	GKEClusterName string `json:"gke_cluster_name"`
}

var GCP gcp

func PodNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns
		}
	}
	return corev1.NamespaceDefault
}

var GrafanaEnabled bool
