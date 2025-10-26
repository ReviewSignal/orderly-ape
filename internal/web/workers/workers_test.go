// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package workers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("parseNodeSelector", func() {
	It("should handle empty string", func() {
		result := parseNodeSelector("")
		Expect(result).To(Equal(map[string]string{}))
	})

	It("should parse single key-value pair", func() {
		result := parseNodeSelector("cloud.google.com/gke-spot=true")
		Expect(result).To(Equal(map[string]string{
			"cloud.google.com/gke-spot": "true",
		}))
	})

	It("should parse multiple key-value pairs", func() {
		result := parseNodeSelector("cloud.google.com/gke-spot=true,kubernetes.io/arch=amd64")
		Expect(result).To(Equal(map[string]string{
			"cloud.google.com/gke-spot": "true",
			"kubernetes.io/arch":        "amd64",
		}))
	})

	It("should parse key without value", func() {
		result := parseNodeSelector("my-custom-label,kubernetes.io/arch=amd64")
		Expect(result).To(Equal(map[string]string{
			"my-custom-label":    "",
			"kubernetes.io/arch": "amd64",
		}))
	})

	It("should handle spaces around commas", func() {
		result := parseNodeSelector("cloud.google.com/gke-spot=true, kubernetes.io/arch=amd64")
		Expect(result).To(Equal(map[string]string{
			"cloud.google.com/gke-spot": "true",
			"kubernetes.io/arch":        "amd64",
		}))
	})

	It("should NOT accept spaces around equals sign", func() {
		result := parseNodeSelector("key1 = value1")
		// This should parse as key "key1 " with value " value1" (preserving spaces)
		Expect(result).To(HaveKey("key1 "))
		Expect(result["key1 "]).To(Equal(" value1"))
	})
})

var _ = Describe("formatNodeSelector", func() {
	It("should handle empty map", func() {
		result := formatNodeSelector(map[string]string{})
		Expect(result).To(Equal(""))
	})

	It("should format single key-value pair", func() {
		result := formatNodeSelector(map[string]string{
			"cloud.google.com/gke-spot": "true",
		})
		Expect(result).To(Equal("cloud.google.com/gke-spot=true"))
	})

	It("should format key without value", func() {
		result := formatNodeSelector(map[string]string{
			"my-custom-label": "",
		})
		Expect(result).To(Equal("my-custom-label"))
	})
})

var _ = Describe("parseTolerations", func() {
	It("should handle empty string", func() {
		result := parseTolerations("")
		Expect(result).To(BeNil())
	})

	It("should parse single toleration with effect", func() {
		result := parseTolerations("cloud.google.com/gke-spot=true:NoSchedule")
		Expect(result).To(HaveLen(1))
		Expect(result[0].Key).To(Equal("cloud.google.com/gke-spot"))
		Expect(result[0].Operator).To(Equal(corev1.TolerationOpEqual))
		Expect(result[0].Value).To(Equal("true"))
		Expect(result[0].Effect).To(Equal(corev1.TaintEffectNoSchedule))
	})

	It("should parse multiple tolerations", func() {
		result := parseTolerations("cloud.google.com/gke-spot=true:NoSchedule,kubernetes.io/arch=amd64")
		Expect(result).To(HaveLen(2))

		Expect(result[0].Key).To(Equal("cloud.google.com/gke-spot"))
		Expect(result[0].Operator).To(Equal(corev1.TolerationOpEqual))
		Expect(result[0].Value).To(Equal("true"))
		Expect(result[0].Effect).To(Equal(corev1.TaintEffectNoSchedule))

		Expect(result[1].Key).To(Equal("kubernetes.io/arch"))
		Expect(result[1].Operator).To(Equal(corev1.TolerationOpEqual))
		Expect(result[1].Value).To(Equal("amd64"))
		Expect(result[1].Effect).To(Equal(corev1.TaintEffect("")))
	})

	It("should parse key without value", func() {
		result := parseTolerations("my-custom-label,kubernetes.io/arch=amd64")
		Expect(result).To(HaveLen(2))

		Expect(result[0].Key).To(Equal("my-custom-label"))
		Expect(result[0].Operator).To(Equal(corev1.TolerationOpEqual))
		Expect(result[0].Value).To(Equal(""))
		Expect(result[0].Effect).To(Equal(corev1.TaintEffect("")))

		Expect(result[1].Key).To(Equal("kubernetes.io/arch"))
		Expect(result[1].Operator).To(Equal(corev1.TolerationOpEqual))
		Expect(result[1].Value).To(Equal("amd64"))
		Expect(result[1].Effect).To(Equal(corev1.TaintEffect("")))
	})

	It("should handle spaces around commas", func() {
		result := parseTolerations("cloud.google.com/gke-spot=true:NoSchedule, kubernetes.io/arch=amd64")
		Expect(result).To(HaveLen(2))

		Expect(result[0].Key).To(Equal("cloud.google.com/gke-spot"))
		Expect(result[0].Value).To(Equal("true"))
		Expect(result[0].Effect).To(Equal(corev1.TaintEffectNoSchedule))

		Expect(result[1].Key).To(Equal("kubernetes.io/arch"))
		Expect(result[1].Value).To(Equal("amd64"))
	})

	It("should NOT accept spaces around equals or colon", func() {
		result := parseTolerations("key1 = value1 : NoSchedule")
		Expect(result).To(HaveLen(1))
		// Spaces are preserved, so key becomes "key1 " and value becomes " value1 "
		Expect(result[0].Key).To(Equal("key1 "))
		Expect(result[0].Value).To(Equal(" value1 "))
		Expect(result[0].Effect).To(Equal(corev1.TaintEffect(" NoSchedule")))
	})
})

var _ = Describe("formatTolerations", func() {
	It("should handle empty slice", func() {
		result := formatTolerations([]corev1.Toleration{})
		Expect(result).To(Equal(""))
	})

	It("should format single toleration with effect", func() {
		result := formatTolerations([]corev1.Toleration{
			{
				Key:      "cloud.google.com/gke-spot",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
		})
		Expect(result).To(Equal("cloud.google.com/gke-spot=true:NoSchedule"))
	})

	It("should format multiple tolerations", func() {
		result := formatTolerations([]corev1.Toleration{
			{
				Key:      "cloud.google.com/gke-spot",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			{
				Key:      "kubernetes.io/arch",
				Operator: corev1.TolerationOpEqual,
				Value:    "amd64",
				Effect:   "",
			},
		})
		Expect(result).To(Equal("cloud.google.com/gke-spot=true:NoSchedule,kubernetes.io/arch=amd64"))
	})

	It("should format key without value", func() {
		result := formatTolerations([]corev1.Toleration{
			{
				Key:      "my-custom-label",
				Operator: corev1.TolerationOpEqual,
				Value:    "",
				Effect:   "",
			},
		})
		Expect(result).To(Equal("my-custom-label"))
	})
})
