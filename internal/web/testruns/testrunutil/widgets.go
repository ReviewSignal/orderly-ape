// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testrunutil

import (
	"github.com/a-h/templ"

	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
)

func TestLocationsWidget(nameChoicer forms.Choicer) forms.FieldComponent {
	return func(f *forms.Field, props ...forms.HTMLBaseProps) templ.Component {
		return testRunLocations(f, nameChoicer)
	}
}
