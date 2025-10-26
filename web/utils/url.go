// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package utils

import (
	"fmt"
	"net/url"
)

// SetQueryParams sets multiple query parameters in the provided URL.
// Accepts a map of key-value pairs where values can be of any type.
// Boolean `true` adds the parameter without a value, `false` or `nil` removes the parameter.
// Returns an empty string for invalid URLs.
func SetQueryParams(URL string, params map[string]any) string {
	parsedURL, err := url.Parse(URL)
	if err != nil {
		// Return an empty string for invalid URLs
		return ""
	}

	query := parsedURL.Query()
	for key, value := range params {
		switch v := value.(type) {
		case bool:
			if v {
				query.Set(key, "") // Add parameter without a value
			} else {
				query.Del(key) // Remove parameter
			}
		case nil:
			query.Del(key) // Remove parameter
		default:
			query.Set(key, fmt.Sprintf("%v", v)) // Convert value to string
		}
	}
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String()
}
