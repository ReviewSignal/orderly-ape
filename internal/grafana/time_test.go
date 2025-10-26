// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package grafana

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RoundToNearest5Minutes", func() {
	It("should round down at 2 minutes", func() {
		input := time.Date(2025, 1, 1, 10, 2, 30, 0, time.UTC)
		result := RoundToNearest5Minutes(input, false)
		expected := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		Expect(result).To(Equal(expected))
	})

	It("should round up at 2 minutes", func() {
		input := time.Date(2025, 1, 1, 10, 2, 30, 0, time.UTC)
		result := RoundToNearest5Minutes(input, true)
		expected := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)
		Expect(result).To(Equal(expected))
	})

	It("should round down at 5 minutes (exact)", func() {
		input := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)
		result := RoundToNearest5Minutes(input, false)
		expected := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)
		Expect(result).To(Equal(expected))
	})

	It("should round up at 5 minutes (exact)", func() {
		input := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)
		result := RoundToNearest5Minutes(input, true)
		expected := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)
		Expect(result).To(Equal(expected))
	})

	It("should round down at 7 minutes", func() {
		input := time.Date(2025, 1, 1, 10, 7, 45, 0, time.UTC)
		result := RoundToNearest5Minutes(input, false)
		expected := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)
		Expect(result).To(Equal(expected))
	})

	It("should round up at 7 minutes", func() {
		input := time.Date(2025, 1, 1, 10, 7, 45, 0, time.UTC)
		result := RoundToNearest5Minutes(input, true)
		expected := time.Date(2025, 1, 1, 10, 10, 0, 0, time.UTC)
		Expect(result).To(Equal(expected))
	})
})
