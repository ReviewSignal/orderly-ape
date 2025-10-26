// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

func TestDashboardLink(t *testing.T) {
	tests := []struct {
		name         string
		dashboardUID string
		startTime    *metav1.Time
		wantContains []string
	}{
		{
			name:         "empty UID returns empty link",
			dashboardUID: "",
			wantContains: nil,
		},
		{
			name:         "with UID constructs URL with grafana proxy path",
			dashboardUID: "test-dashboard-uid",
			wantContains: []string{"/grafana/d/test-dashboard-uid", "kiosk="},
		},
		{
			name:         "with start time includes timestamp",
			dashboardUID: "test-uid",
			startTime:    &metav1.Time{Time: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
			wantContains: []string{"/grafana/d/test-uid", "from=1704110100000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := TestRun{
				&apev1.TestRun{
					Status: apev1.TestRunStatus{
						DashboardUID: tt.dashboardUID,
						StartTime:    tt.startTime,
					},
				},
			}

			got := tr.GrafanaEmbededDashboardLink()

			if tt.wantContains == nil && got != "" {
				t.Errorf("DashboardLink() = %v, want empty string", got)
				return
			}

			for _, want := range tt.wantContains {
				if got == "" || len(got) == 0 {
					t.Errorf("DashboardLink() = %v, want to contain %v", got, want)
					return
				}
				// Simple substring check - in a real scenario you might want to parse the URL
				contains := false
				for i := 0; i <= len(got)-len(want); i++ {
					if got[i:i+len(want)] == want {
						contains = true
						break
					}
				}
				if !contains {
					t.Errorf("DashboardLink() = %v, want to contain %v", got, want)
				}
			}
		})
	}
}
