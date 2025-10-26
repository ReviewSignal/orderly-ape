// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package jwt_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestJWT(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "JWT Suite")
}
