// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testscenarios

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTestScenarios(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TestScenarios Suite")
}
