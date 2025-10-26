// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package forms

import (
	"errors"
	"fmt"
)

// Common errors
var (
	ErrOutOfRange = errors.New("out of range")
	ErrRequired   = errors.New("required")
	ErrInvalid    = errors.New("invalid value")
)

// FieldError represents a custom error type for field validation
type FieldError struct {
	UserMessage string
	Err         error
}

func (e *FieldError) Error() string {
	return e.UserMessage
}

func (e *FieldError) Unwrap() error {
	return e.Err
}

// FieldErrorf creates a new FieldError with a formatted user message and an underlying error.
func FieldErrorf(err error, format string, a ...any) error {
	return &FieldError{
		UserMessage: fmt.Sprintf(format, a...),
		Err:         err,
	}
}
