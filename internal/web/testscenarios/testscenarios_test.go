// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testscenarios

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/internal/web/testruns/testrunutil"
)

var _ = Describe("TestScenariosController", func() {
	Context("filterTestScenarios", func() {
		It("should be implemented", func() {
			// Basic test to ensure the controller is set up correctly
			controller := &TestScenariosController{}
			Expect(controller).NotTo(BeNil())
		})
	})

	Context("ParseLocations", func() {
		It("should parse valid locations", func() {
			form := &forms.Form{}
			data := map[string]string{
				"locations": `[{"name":"default/worker-1","workers":2}]`,
			}

			workers, err := testrunutil.ParseLocations(data, form)

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

			workers, err := testrunutil.ParseLocations(data, form)

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

			envVars, err := testrunutil.ParseEnvVars(data, form)

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

			envVars, err := testrunutil.ParseEnvVars(data, form)

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

			labels, err := testrunutil.ParseLabels(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(HaveLen(1))
			Expect(labels["app"]).To(Equal("test"))
		})

		It("should filter out empty names", func() {
			form := &forms.Form{}
			data := map[string]string{
				"labels": `[{"name":"app","value":"test"},{"name":"","value":"empty"}]`,
			}

			labels, err := testrunutil.ParseLabels(data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(HaveLen(1))
			Expect(labels["app"]).To(Equal("test"))
		})
	})

	Context("parseResourceQuantity", func() {
		It("should parse valid CPU quantity", func() {
			quantity, err := parseResourceQuantity("1")

			Expect(err).NotTo(HaveOccurred())
			Expect(quantity.String()).To(Equal("1"))
		})

		It("should parse valid CPU quantity with millicores", func() {
			quantity, err := parseResourceQuantity("500m")

			Expect(err).NotTo(HaveOccurred())
			Expect(quantity.MilliValue()).To(Equal(int64(500)))
		})

		It("should parse valid memory quantity with Gi", func() {
			quantity, err := parseResourceQuantity("2Gi")

			Expect(err).NotTo(HaveOccurred())
			Expect(quantity.String()).To(Equal("2Gi"))
		})

		It("should return error for invalid quantity", func() {
			_, err := parseResourceQuantity("invalid")

			Expect(err).To(HaveOccurred())
		})
	})

	Context("updateResourceLimits", func() {
		It("should set CPU and memory limits", func() {
			spec := &apev1.TestScenarioSpec{}
			form := &forms.Form{}
			data := map[string]string{
				"cpuLimit":    "2",
				"memoryLimit": "4Gi",
			}

			err := updateResourceLimits(spec, data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(spec.Resources).NotTo(BeNil())
			Expect(spec.Resources.Limits).NotTo(BeNil())
			cpuLimit := spec.Resources.Limits[corev1.ResourceCPU]
			memLimit := spec.Resources.Limits[corev1.ResourceMemory]
			Expect(cpuLimit.String()).To(Equal("2"))
			Expect(memLimit.String()).To(Equal("4Gi"))
		})

		It("should handle empty values", func() {
			spec := &apev1.TestScenarioSpec{}
			form := &forms.Form{}
			data := map[string]string{
				"cpuLimit":    "",
				"memoryLimit": "",
			}

			err := updateResourceLimits(spec, data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(spec.Resources).To(BeNil())
		})

		It("should clear CPU when only CPU is empty", func() {
			spec := &apev1.TestScenarioSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			}
			form := &forms.Form{}
			data := map[string]string{
				"cpuLimit":    "",
				"memoryLimit": "2Gi",
			}

			err := updateResourceLimits(spec, data, form)

			Expect(err).NotTo(HaveOccurred())
			Expect(spec.Resources).NotTo(BeNil())
			Expect(spec.Resources.Limits).NotTo(BeNil())
			_, hasCPU := spec.Resources.Limits[corev1.ResourceCPU]
			Expect(hasCPU).To(BeFalse())
			memLimit := spec.Resources.Limits[corev1.ResourceMemory]
			Expect(memLimit.String()).To(Equal("2Gi"))
		})

		It("should return error for invalid CPU", func() {
			spec := &apev1.TestScenarioSpec{}
			form := &forms.Form{}
			data := map[string]string{
				"cpuLimit":    "invalid",
				"memoryLimit": "2Gi",
			}

			err := updateResourceLimits(spec, data, form)

			Expect(err).To(HaveOccurred())
		})

		It("should return error for invalid memory", func() {
			spec := &apev1.TestScenarioSpec{}
			form := &forms.Form{}
			data := map[string]string{
				"cpuLimit":    "1",
				"memoryLimit": "invalid",
			}

			err := updateResourceLimits(spec, data, form)

			Expect(err).To(HaveOccurred())
		})
	})
})
