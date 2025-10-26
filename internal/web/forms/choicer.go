// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package forms

type Choice struct {
	Label  string
	Value  string
	Object any
}

type Choicer interface {
	List() []Choice
	IsMultivalue() bool
}

type ChoiceFunc func() []Choice

func (f ChoiceFunc) List() []Choice {
	return f()
}

func (f ChoiceFunc) IsMultivalue() bool {
	return false
}

type MultiChoiceFunc func() []Choice

func (f MultiChoiceFunc) List() []Choice {
	return f()
}

func (f MultiChoiceFunc) IsMultivalue() bool {
	return true
}

const ChoicerDefaultFallbackFirst = ""

func ChoicerDefault(c Choicer, d string, defaults ...string) string {
	items := c.List()

	if d != ChoicerDefaultFallbackFirst {
		for _, c := range items {
			if c.Value == d {
				return c.Value
			}
		}
	}

	for _, def := range defaults {
		for _, c := range items {
			if def == c.Value {
				return def
			}
		}
	}

	if d == ChoicerDefaultFallbackFirst {
		if len(items) > 0 {
			return items[0].Value
		} else {
			return ""
		}
	}

	return ""
}

func ObjectByChoice[T any](c Choicer, choice string) (T, bool) {
	var zero T
	for _, item := range c.List() {
		if item.Value == choice {
			if obj, ok := item.Object.(T); ok {
				return obj, true
			}
			return zero, false
		}
	}
	return zero, false
}
