// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package workers

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPodPlacements(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workers Suite")
}
