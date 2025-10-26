// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package utils

import (
	"fmt"
	"net/url"

	"github.com/a-h/templ"

	"github.com/ReviewSignal/orderly-ape/internal/info"
)

func AuditLogsURL() templ.SafeURL {
	query := map[string]string{
		"protoPayload.@type":           "type.googleapis.com/google.cloud.audit.AuditLog",
		"resource.labels.cluster_name": info.GCP.GKEClusterName,
	}

	auditLogsQuery := ""
	for k, v := range query {
		auditLogsQuery += fmt.Sprintf("%s=\"%s\" ", k, v)
	}

	return templ.URL(fmt.Sprintf(
		"https://console.cloud.google.com/logs/query;query=%s;duration=PT1H?project=%s",
		url.PathEscape(auditLogsQuery),
		info.GCP.ProjectID,
	))

}
