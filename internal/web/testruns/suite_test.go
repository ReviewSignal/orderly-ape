// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testruns

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTestRuns(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TestRuns Suite")
}
