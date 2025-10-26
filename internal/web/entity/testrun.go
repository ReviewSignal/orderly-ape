// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"fmt"
	"net/url"
	"time"

	"github.com/a-h/templ"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/grafana"
	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
	"github.com/ReviewSignal/orderly-ape/web/utils"
)

type TestRun struct {
	*apev1.TestRun
}

type TestRunList []TestRun

func WrapTestRun(t *apev1.TestRun) TestRun {
	return TestRun{t}
}

func WrapTestRunList(items []apev1.TestRun) TestRunList {
	wrapped := make(TestRunList, len(items))
	for i := range items {
		wrapped[i] = WrapTestRun(&items[i])
	}
	return wrapped
}

func (t TestRun) NamespacedName() string {
	return client.ObjectKeyFromObject(t.TestRun).String()
}

func (t TestRun) ObjectLink() templ.SafeURL {
	path, _ := url.JoinPath("testruns", t.Name)
	return templ.URL(path)
}

func (t TestRun) VerboseName() string {
	return "Test Run"
}

func (t TestRun) GrafanaEmbededDashboardLink() string {
	if t.Status.DashboardUID == "" {
		return ""
	}

	// Construct the dashboard URL from the UID using the grafana proxy path
	url := fmt.Sprintf("/grafana/d/%s", t.Status.DashboardUID)

	params := map[string]any{
		"from":  "now-1h",
		"to":    "now",
		"kiosk": "true",
		"theme": "system",
	}

	if !t.Status.StartTime.IsZero() {
		// Round start time down to nearest 5-minute interval, then subtract 5 minutes for buffer
		roundedStart := grafana.RoundToNearest5Minutes(t.Status.StartTime.Time, false)
		params["from"] = roundedStart.Add(-5 * time.Minute).UnixMilli()
	}

	if t.Status.CompletionTime.IsZero() { // still running
		params["refresh"] = "10s"
	} else { // completed
		// Round completion time up to nearest 5-minute interval
		roundedEnd := grafana.RoundToNearest5Minutes(t.Status.CompletionTime.Time, true)
		params["to"] = roundedEnd.UnixMilli()
	}

	return utils.SetQueryParams(url, params)
}

func (t TestRun) EntryStatus() datatable.Status {
	if !t.GetDeletionTimestamp().IsZero() {
		return datatable.Status{
			Icon:        datatable.StatusIconDeleteing,
			Description: "TestRun is being deleted",
		}
	}
	phase := t.Phase()
	st := datatable.Status{
		Title:                phase.String(),
		Description:          phase.String(),
		DescriptionComponent: t.TestRunStatus,
	}

	switch phase {
	case apev1.TestRunPhaseDraft:
		st.Icon = datatable.StatusIconDraft
	case apev1.TestRunPhaseFailed:
		st.Icon = datatable.StatusIconFailed
	case apev1.TestRunPhaseCanceling:
		st.Icon = datatable.StatusIconCanceling
	case apev1.TestRunPhaseCanceled:
		st.Icon = datatable.StatusIconCanceled
	case apev1.TestRunPhaseCompleted:
		st.Icon = datatable.StatusIconCompleted
	case apev1.TestRunPhasePending, apev1.TestRunPhaseReady, apev1.TestRunPhaseQueued:
		st.Icon = datatable.StatusIconPending
	case apev1.TestRunPhaseRunning:
		st.Icon = datatable.StatusIconRunning
	case apev1.TestRunPhaseArchived:
		st.Icon = datatable.StatusIconArchived
	default:
		st.Icon = datatable.StatusIconUnknown
	}

	return st
}
