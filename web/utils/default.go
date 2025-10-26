// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package utils

import "fmt"

func Default[T any](value *T, defaultValue string) string {
	if value == nil {
		return defaultValue
	}
	return fmt.Sprintf("%v", *value)
}
