// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package grafana

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

//go:embed all:dashboards
var dashboards embed.FS

func jsonEscape(i string) string {
	b, err := json.Marshal(i)
	if err != nil {
		panic(err)
	}
	// Trim the beginning and trailing " character
	return string(b[1 : len(b)-1])
}

// RoundToNearest5Minutes rounds a time to the nearest 5-minute interval.
// When roundUp is false, it rounds down to the previous 5-minute mark.
// When roundUp is true, it rounds up to the next 5-minute mark (unless already at a 5-minute mark).
// Examples:
//   - RoundToNearest5Minutes(10:02:30, false) => 10:00:00
//   - RoundToNearest5Minutes(10:02:30, true) => 10:05:00
//   - RoundToNearest5Minutes(10:05:00, false) => 10:05:00
//   - RoundToNearest5Minutes(10:05:00, true) => 10:05:00
func RoundToNearest5Minutes(t time.Time, roundUp bool) time.Time {
	// Truncate to the start of the minute
	t = t.Truncate(time.Minute)

	// Get the minute component
	minute := t.Minute()

	// Calculate the remainder when dividing by 5
	remainder := minute % 5

	if roundUp && remainder != 0 {
		// Round up to the next 5-minute interval
		t = t.Add(time.Duration(5-remainder) * time.Minute)
	} else {
		// Round down to the previous 5-minute interval
		t = t.Add(-time.Duration(remainder) * time.Minute)
	}

	return t
}

// PrepareDashboard loads the test-results.json template and prepares it for a specific test run
func PrepareDashboard(testName, testTarget, bucket, dsInfluxUID, dsVictoriaUID, version string, startTime, endTime *time.Time, aggregationInterval int, tags []string) (map[string]any, error) {
	// Read the embedded dashboard template
	data, err := dashboards.ReadFile("dashboards/test-results.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read dashboard template: %w", err)
	}

	dashboardJSON := string(data)
	dashboardJSON = strings.ReplaceAll(dashboardJSON, "${DS_INFLUXDB}", dsInfluxUID)
	dashboardJSON = strings.ReplaceAll(dashboardJSON, "${DS_VICTORIALOGS}", dsVictoriaUID)
	dashboardJSON = strings.ReplaceAll(dashboardJSON, "${TEST_ID}", testName)
	dashboardJSON = strings.ReplaceAll(dashboardJSON, "${TEST_TARGET}", jsonEscape(testTarget))
	dashboardJSON = strings.ReplaceAll(dashboardJSON, "${INFLUXDB_BUCKET}", bucket)
	dashboardJSON = strings.ReplaceAll(dashboardJSON, "${AGGREGATION_INTERVAL}", fmt.Sprintf("%ds", aggregationInterval))
	dashboardJSON = strings.ReplaceAll(dashboardJSON, "${VERSION}", version)

	// Parse the JSON template
	var dashboard map[string]any
	if err := json.Unmarshal([]byte(dashboardJSON), &dashboard); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard template: %w", err)
	}

	// Remove uid field (Grafana generates new one)
	delete(dashboard, "uid")

	// Update title to include test name
	dashboard["title"] = fmt.Sprintf("Test Results - %s", testName)

	// Add tags to the dashboard (convert []string to []any for JSON compatibility)
	tagsAny := make([]any, len(tags))
	for i, tag := range tags {
		tagsAny[i] = tag
	}
	dashboard["tags"] = tagsAny

	// Set time range if provided
	if startTime != nil || endTime != nil {
		timeObj := make(map[string]any)

		if startTime != nil {
			// Round start time down to nearest 5-minute interval
			rounded := RoundToNearest5Minutes(*startTime, false)
			timeObj["from"] = rounded.UTC().Format(time.RFC3339)
		} else {
			timeObj["from"] = "now-1h"
		}

		if endTime != nil {
			// Round end time up to nearest 5-minute interval
			rounded := RoundToNearest5Minutes(*endTime, true)
			timeObj["to"] = rounded.UTC().Format(time.RFC3339)
		} else {
			timeObj["to"] = "now"
		}

		dashboard["time"] = timeObj
	}

	return dashboard, nil
}
