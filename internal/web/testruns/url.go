// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testruns

import (
	"net/url"
	"regexp"
	"strings"
)

// FromURL converts a URL to a DNS-safe prefix suitable for GenerateName.
// The prefix is generated from the hostname and path of the URL.
// The function:
// - Extracts hostname and path from the URL
// - Removes protocol, query parameters, and fragments
// - Replaces dots and slashes with dashes
// - Removes non-alphanumeric characters except dashes
// - Removes consecutive dashes
// - Limits to 31 characters to leave room for trailing dash
// - Ensures it ends with exactly one dash
// Example: https://quickpizza.grafana.com/contacts.php → quickpizza-grafana-com-contacts-php-
func FromURL(rawURL string) string {
	const maxLength = 32 // including trailing dash

	// Parse the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" {
		// If URL is invalid, return a default prefix
		return "test-run-"
	}

	// Combine hostname and path
	var combined string

	// Add hostname
	hostname := parsedURL.Host
	// Remove port if present
	if colonIdx := strings.Index(hostname, ":"); colonIdx != -1 {
		hostname = hostname[:colonIdx]
	}
	combined = hostname

	// Add path (without leading slash)
	if parsedURL.Path != "" && parsedURL.Path != "/" {
		path := strings.Trim(parsedURL.Path, "/")
		combined = combined + "/" + path
	}

	// Replace dots and slashes with dashes
	combined = strings.ReplaceAll(combined, ".", "-")
	combined = strings.ReplaceAll(combined, "/", "-")
	// Replace underscores with dashes
	combined = strings.ReplaceAll(combined, "_", "-")

	// Convert to lowercase
	combined = strings.ToLower(combined)

	// Remove non-alphanumeric characters except dashes (after lowercasing)
	re := regexp.MustCompile("[^a-z0-9-]")
	combined = re.ReplaceAllString(combined, "")

	// Remove consecutive dashes
	re = regexp.MustCompile("-+")
	combined = re.ReplaceAllString(combined, "-")

	// Trim leading and trailing dashes
	combined = strings.Trim(combined, "-")

	// If empty after cleaning, return default
	if combined == "" {
		return "test-run-"
	}

	// Limit to 31 characters to leave room for trailing dash
	if len(combined) > maxLength-1 {
		combined = combined[:maxLength-1]
	}

	// Ensure no trailing dash before adding our own
	combined = strings.TrimSuffix(combined, "-")

	// Add trailing dash
	return combined + "-"
}
