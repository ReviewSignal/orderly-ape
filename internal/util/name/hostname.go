// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package name

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"strings"
)

// FromHostname converts a domain name to the format PREFIX-HASH
// HASH is a 7-character SHA-1 hash of the domain.
// PREFIX is the domain with dots replaced by -, avoiding double dashes.
// If the domain starts with '*', it is replaced with 'all'.
// If the resulting prefix is longer than maxWidth, it is truncated.
func FromHostname(domain string, maxWidth int, salts ...string) string {
	const hashLength = 7

	// Compute the SHA-1 hash of the domain
	hasher := sha1.New()
	hasher.Write([]byte(domain))
	for _, salt := range salts {
		hasher.Write([]byte(salt))
	}
	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	// If the hash length is greater than maxWidth, return the hash only
	// Take into account the separator '-' between prefix and hash
	if maxWidth < hashLength+1 {
		return hash[:min(maxWidth, hashLength)]
	}

	prefixMaxWidth := maxWidth - hashLength

	// Replace '*' at the start with 'all'
	if strings.HasPrefix(domain, "*") {
		domain = strings.Replace(domain, "*", "all", 1)
	}

	// Replace dots with dashes
	prefix := strings.ReplaceAll(domain, ".", "-")

	// Replace multiple consecutive dashes with a single dash using regex
	re := regexp.MustCompile("-+")
	prefix = re.ReplaceAllString(prefix, "-")

	// Trim to the desired maximum width if necessary
	if len(prefix) > maxWidth {
		prefix = prefix[:prefixMaxWidth]
	}

	// Remove trailing dash if it exists
	prefix = strings.TrimSuffix(prefix, "-")

	// Return the formatted string
	return fmt.Sprintf("%s-%s", prefix, hash[:hashLength])
}
