// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testruns

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FromURL", func() {
	It("should generate prefix from URL with hostname and path", func() {
		result := FromURL("https://quickpizza.grafana.com/contacts.php")
		// This exceeds 32 chars, so it will be truncated to 31 chars + trailing dash
		Expect(result).To(Equal("quickpizza-grafana-com-contacts-"))
		Expect(result).To(HaveLen(32))
	})

	It("should generate prefix from URL with only hostname", func() {
		result := FromURL("https://example.com")
		Expect(result).To(Equal("example-com-"))
	})

	It("should generate prefix from URL with subdomain", func() {
		result := FromURL("https://www.example.com")
		Expect(result).To(Equal("www-example-com-"))
	})

	It("should handle URL with port", func() {
		result := FromURL("https://example.com:8080/path")
		Expect(result).To(Equal("example-com-path-"))
	})

	It("should handle URL with multiple path segments", func() {
		result := FromURL("https://example.com/path/to/resource")
		Expect(result).To(Equal("example-com-path-to-resource-"))
	})

	It("should remove query parameters", func() {
		result := FromURL("https://example.com/path?query=value")
		Expect(result).To(Equal("example-com-path-"))
	})

	It("should remove fragments", func() {
		result := FromURL("https://example.com/path#fragment")
		Expect(result).To(Equal("example-com-path-"))
	})

	It("should handle uppercase letters", func() {
		result := FromURL("https://Example.COM/Path")
		Expect(result).To(Equal("example-com-path-"))
	})

	It("should remove special characters", func() {
		result := FromURL("https://example.com/path_with-special$chars!")
		// This exceeds 32 chars, so it will be truncated
		Expect(result).To(Equal("example-com-path-with-specialch-"))
		Expect(result).To(HaveLen(32))
	})

	It("should collapse consecutive dashes", func() {
		result := FromURL("https://example.com/path--with---dashes")
		Expect(result).To(Equal("example-com-path-with-dashes-"))
	})

	It("should truncate long URLs to 32 characters", func() {
		result := FromURL("https://very-long-subdomain-name.example.com/very/long/path/to/resource/file")
		Expect(result).To(HaveLen(32))
		Expect(result).To(HaveSuffix("-"))
	})

	It("should return default prefix for invalid URL", func() {
		result := FromURL("not-a-valid-url")
		Expect(result).To(Equal("test-run-"))
	})

	It("should return default prefix for empty URL", func() {
		result := FromURL("")
		Expect(result).To(Equal("test-run-"))
	})

	It("should handle URL with only slash as path", func() {
		result := FromURL("https://example.com/")
		Expect(result).To(Equal("example-com-"))
	})

	It("should handle URL with trailing slash in path", func() {
		result := FromURL("https://example.com/path/")
		Expect(result).To(Equal("example-com-path-"))
	})

	It("should end with exactly one dash", func() {
		result := FromURL("https://example.com/path")
		Expect(result).To(HaveSuffix("-"))
		Expect(result).NotTo(HaveSuffix("--"))
	})

	It("should not have consecutive dashes", func() {
		result := FromURL("https://example.com/path.to.resource")
		Expect(result).NotTo(ContainSubstring("--"))
	})

	It("should handle URL with file extension", func() {
		result := FromURL("https://example.com/index.html")
		Expect(result).To(Equal("example-com-index-html-"))
	})

	It("should handle complex real-world URL", func() {
		result := FromURL("https://api.github.com/repos/owner/repo")
		// This is exactly 32 chars
		Expect(result).To(Equal("api-github-com-repos-owner-repo-"))
		Expect(result).To(HaveLen(32))
	})

	It("should handle short URL that fits within limit", func() {
		result := FromURL("https://test.io/api")
		Expect(result).To(Equal("test-io-api-"))
		Expect(result).To(HaveLen(12))
	})
})
