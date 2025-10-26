// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testruns

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/web/entity"
)

var _ = Describe("filterTestRuns", func() {
	It("should filter by name", func() {
		testruns := []apev1.TestRun{
			{Spec: apev1.TestRunSpec{Target: "https://example.com"}},
		}
		testruns[0].Name = "test-run-123"

		req, _ := http.NewRequest("GET", "/?s=test-run-123", nil)
		result := filterTestRuns(testruns, req)
		Expect(result).To(HaveLen(1))
	})

	It("should filter by target", func() {
		testruns := []apev1.TestRun{
			{Spec: apev1.TestRunSpec{Target: "https://example.com"}},
			{Spec: apev1.TestRunSpec{Target: "https://other.com"}},
		}
		testruns[0].Name = "test-run-1"
		testruns[1].Name = "test-run-2"

		req, _ := http.NewRequest("GET", "/?s=example", nil)
		result := filterTestRuns(testruns, req)
		Expect(result).To(HaveLen(1))
		Expect(result[0].Spec.Target).To(Equal("https://example.com"))
	})

	It("should return all when no search term", func() {
		testruns := []apev1.TestRun{
			{Spec: apev1.TestRunSpec{Target: "https://example.com"}},
			{Spec: apev1.TestRunSpec{Target: "https://other.com"}},
		}
		testruns[0].Name = "test-run-1"
		testruns[1].Name = "test-run-2"

		req, _ := http.NewRequest("GET", "/", nil)
		result := filterTestRuns(testruns, req)
		Expect(result).To(HaveLen(2))
	})
})

var _ = Describe("TestRun entity", func() {
	It("should wrap TestRun correctly", func() {
		tr := &apev1.TestRun{}
		tr.Name = "test-run-123"
		tr.Namespace = "default"

		wrapped := entity.WrapTestRun(tr)
		Expect(wrapped.GetName()).To(Equal("test-run-123"))
		Expect(wrapped.GetNamespace()).To(Equal("default"))
		Expect(wrapped.VerboseName()).To(Equal("Test Run"))
	})

	It("should wrap TestRun list correctly", func() {
		testruns := []apev1.TestRun{
			{Spec: apev1.TestRunSpec{Target: "https://example.com"}},
			{Spec: apev1.TestRunSpec{Target: "https://other.com"}},
		}
		testruns[0].Name = "test-run-1"
		testruns[1].Name = "test-run-2"

		wrapped := entity.WrapTestRunList(testruns)
		Expect(wrapped).To(HaveLen(2))
		Expect(wrapped[0].GetName()).To(Equal("test-run-1"))
		Expect(wrapped[1].GetName()).To(Equal("test-run-2"))
	})

	It("should support Workers field", func() {
		tr := &apev1.TestRun{
			Spec: apev1.TestRunSpec{
				Target: "https://example.com",
				Workers: []apev1.TestRunWorkerSpec{
					{ObjectReference: apev1.ObjectReference{Name: "worker-1", Namespace: "default"}},
					{ObjectReference: apev1.ObjectReference{Name: "worker-2", Namespace: "default"}},
				},
			},
		}
		tr.Name = "test-run-123"
		tr.Namespace = "default"

		Expect(tr.Spec.Workers).To(HaveLen(2))
		Expect(tr.Spec.Workers[0].Name).To(Equal("worker-1"))
		Expect(tr.Spec.Workers[1].Name).To(Equal("worker-2"))
	})
})

var _ = Describe("TestRun EditForm", func() {
	It("should create editable form for Draft TestRun", func() {
		tr := &apev1.TestRun{
			Spec: apev1.TestRunSpec{
				Target: "https://example.com",
				State:  apev1.TestRunStateDraft,
				Scenario: &apev1.ObjectReference{
					Name:      "test-scenario",
					Namespace: "default",
				},
				InfluxDB: &apev1.ObjectReference{
					Name:      "test-influxdb",
					Namespace: "default",
				},
			},
		}
		tr.Name = "test-run-123"
		tr.Namespace = "default"

		wrapped := entity.WrapTestRun(tr)

		// Verify that Draft state is editable
		Expect(wrapped.Spec.State).To(Equal(apev1.TestRunStateDraft))
	})

	It("should create read-only form for Active TestRun", func() {
		tr := &apev1.TestRun{
			Spec: apev1.TestRunSpec{
				Target: "https://example.com",
				State:  apev1.TestRunStateActive,
				Scenario: &apev1.ObjectReference{
					Name:      "test-scenario",
					Namespace: "default",
				},
				InfluxDB: &apev1.ObjectReference{
					Name:      "test-influxdb",
					Namespace: "default",
				},
			},
		}
		tr.Name = "test-run-123"
		tr.Namespace = "default"

		wrapped := entity.WrapTestRun(tr)

		// Verify that Active state is not editable
		Expect(wrapped.Spec.State).To(Equal(apev1.TestRunStateActive))
	})

	It("should create read-only form for Canceled TestRun", func() {
		tr := &apev1.TestRun{
			Spec: apev1.TestRunSpec{
				Target: "https://example.com",
				State:  apev1.TestRunStateCanceled,
				Scenario: &apev1.ObjectReference{
					Name:      "test-scenario",
					Namespace: "default",
				},
				InfluxDB: &apev1.ObjectReference{
					Name:      "test-influxdb",
					Namespace: "default",
				},
			},
		}
		tr.Name = "test-run-123"
		tr.Namespace = "default"

		wrapped := entity.WrapTestRun(tr)

		// Verify that Canceled state is not editable
		Expect(wrapped.Spec.State).To(Equal(apev1.TestRunStateCanceled))
	})

	It("should create read-only form for Archived TestRun", func() {
		tr := &apev1.TestRun{
			Spec: apev1.TestRunSpec{
				Target: "https://example.com",
				State:  apev1.TestRunStateArchived,
				Scenario: &apev1.ObjectReference{
					Name:      "test-scenario",
					Namespace: "default",
				},
				InfluxDB: &apev1.ObjectReference{
					Name:      "test-influxdb",
					Namespace: "default",
				},
			},
		}
		tr.Name = "test-run-123"
		tr.Namespace = "default"

		wrapped := entity.WrapTestRun(tr)

		// Verify that Archived state is not editable
		Expect(wrapped.Spec.State).To(Equal(apev1.TestRunStateArchived))
	})
})
