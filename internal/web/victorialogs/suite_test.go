// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package victorialogs_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVictoriaLogsProxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Victoria Logs Proxy Suite")
}
