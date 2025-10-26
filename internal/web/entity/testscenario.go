// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"net/url"

	"github.com/a-h/templ"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

type TestScenario struct {
	*apev1.TestScenario
}

type TestScenarioList []TestScenario

func WrapTestScenario(ts *apev1.TestScenario) TestScenario {
	return TestScenario{ts}
}

func WrapTestScenarioList(items []apev1.TestScenario) TestScenarioList {
	wrapped := make(TestScenarioList, len(items))
	for i := range items {
		wrapped[i] = WrapTestScenario(&items[i])
	}
	return wrapped
}

func (ts TestScenario) NamespacedName() string {
	return client.ObjectKeyFromObject(ts.TestScenario).String()
}

func (ts TestScenario) ObjectLink() templ.SafeURL {
	path, _ := url.JoinPath("testscenarios", ts.Name)
	return templ.URL(path)
}

func (ts TestScenario) VerboseName() string {
	return "Test Scenario"
}
