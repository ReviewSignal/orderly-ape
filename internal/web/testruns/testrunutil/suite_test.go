// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testrunutil

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTestRunUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TestRunUtil Suite")
}
