// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testrunutil

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
)

var _ = Describe("TestRunUtil", func() {
	Context("ParseLocations", func() {
		It("should parse valid locations", func() {
			form := &forms.Form{}
			data := map[string]string{
				"locations": `[{"name":"default/worker-1","workers":2}]`,
			}

			workers, err := ParseLocations(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(workers).To(HaveLen(1))
			Expect(workers[0].Name).To(Equal("worker-1"))
			Expect(workers[0].Namespace).To(Equal("default"))
			Expect(workers[0].NumWorkers).To(Equal(int32(2)))
		})

		It("should return nil for empty locations", func() {
			form := &forms.Form{}
			data := map[string]string{
				"locations": "",
			}

			workers, err := ParseLocations(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(workers).To(BeNil())
		})
	})

	Context("ParseEnvVars", func() {
		It("should parse valid environment variables", func() {
			form := &forms.Form{}
			data := map[string]string{
				"env": `[{"name":"MY_VAR","value":"test"}]`,
			}

			envVars, err := ParseEnvVars(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("MY_VAR"))
			Expect(envVars[0].Value).To(Equal("test"))
		})

		It("should filter out empty names", func() {
			form := &forms.Form{}
			data := map[string]string{
				"env": `[{"name":"MY_VAR","value":"test"},{"name":"","value":"empty"}]`,
			}

			envVars, err := ParseEnvVars(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("MY_VAR"))
		})
	})

	Context("ParseLabels", func() {
		It("should parse valid labels", func() {
			form := &forms.Form{}
			data := map[string]string{
				"labels": `[{"name":"app","value":"test"}]`,
			}

			labels, err := ParseLabels(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(HaveLen(1))
			Expect(labels["app"]).To(Equal("test"))
		})

		It("should filter out empty names", func() {
			form := &forms.Form{}
			data := map[string]string{
				"labels": `[{"name":"app","value":"test"},{"name":"","value":"empty"}]`,
			}

			labels, err := ParseLabels(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(HaveLen(1))
			Expect(labels["app"]).To(Equal("test"))
		})
	})

	Context("MergeEnvVars", func() {
		It("should merge environment variables with testRun having priority", func() {
			scenarioEnvVars := []apev1.EnvVar{
				{Name: "VAR1", Value: "scenario1"},
				{Name: "VAR2", Value: "scenario2"},
			}
			testRunEnvVars := []apev1.EnvVar{
				{Name: "VAR2", Value: "testrun2"},
				{Name: "VAR3", Value: "testrun3"},
			}

			result := MergeEnvVars(scenarioEnvVars, testRunEnvVars)

			Expect(result).To(HaveLen(3))
			// Check that VAR2 from testRun overrides scenario
			var2Found := false
			for _, ev := range result {
				if ev.Name == "VAR2" {
					Expect(ev.Value).To(Equal("testrun2"))
					var2Found = true
				}
			}
			Expect(var2Found).To(BeTrue())
		})

		It("should handle empty scenario envVars", func() {
			testRunEnvVars := []apev1.EnvVar{
				{Name: "VAR1", Value: "testrun1"},
			}

			result := MergeEnvVars(nil, testRunEnvVars)

			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("VAR1"))
			Expect(result[0].Value).To(Equal("testrun1"))
		})

		It("should handle empty testRun envVars", func() {
			scenarioEnvVars := []apev1.EnvVar{
				{Name: "VAR1", Value: "scenario1"},
			}

			result := MergeEnvVars(scenarioEnvVars, nil)

			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("VAR1"))
			Expect(result[0].Value).To(Equal("scenario1"))
		})
	})

	Context("MergeLabels", func() {
		It("should merge labels with testRun having priority", func() {
			scenarioLabels := map[string]string{
				"label1": "scenario1",
				"label2": "scenario2",
			}
			testRunLabels := map[string]string{
				"label2": "testrun2",
				"label3": "testrun3",
			}

			result := MergeLabels(scenarioLabels, testRunLabels)

			Expect(result).To(HaveLen(3))
			Expect(result["label1"]).To(Equal("scenario1"))
			Expect(result["label2"]).To(Equal("testrun2"))
			Expect(result["label3"]).To(Equal("testrun3"))
		})

		It("should handle nil scenario labels", func() {
			testRunLabels := map[string]string{
				"label1": "testrun1",
			}

			result := MergeLabels(nil, testRunLabels)

			Expect(result).To(HaveLen(1))
			Expect(result["label1"]).To(Equal("testrun1"))
		})

		It("should handle nil testRun labels", func() {
			scenarioLabels := map[string]string{
				"label1": "scenario1",
			}

			result := MergeLabels(scenarioLabels, nil)

			Expect(result).To(HaveLen(1))
			Expect(result["label1"]).To(Equal("scenario1"))
		})

		It("should return nil when both are nil", func() {
			result := MergeLabels(nil, nil)
			Expect(result).To(BeNil())
		})
	})
})
