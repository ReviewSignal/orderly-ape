// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package forms

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/ReviewSignal/orderly-ape/web/components/templui/input"
	"github.com/ReviewSignal/orderly-ape/web/utils"
)

type HTMLBaseProps struct {
	ID         string
	Class      string
	Attributes templ.Attributes
}

type FieldComponent func(*Field, ...HTMLBaseProps) templ.Component

type Field struct {
	Name         string
	InitialValue string
	value        *string
	Label        string
	Description  string
	Placeholder  string
	Hidden       bool
	Disabled     bool
	Readonly     bool
	Checked      bool
	InputType    input.Type
	InlineLabel  bool
	Choicer      Choicer

	Required          bool
	MaxLength         int
	MinLength         int
	Min               *int
	Max               *int
	MaxItems          *int
	MultiValue        bool
	Step              *int
	Pattern           string
	PatternInvalidMsg string
	Rows              int
	AutoResize        bool

	WidgetComponent      FieldComponent
	WidgetProps          HTMLBaseProps
	DescriptionComponent FieldComponent
	FormItem             FieldComponent

	Error error

	form           *Form
	id             string
	componentDepth int
}

func (f *Field) SetValue(value string) {
	f.value = &value
}

func (f *Field) Value() string {
	if f == nil {
		return ""
	}
	if f.value != nil {
		return *f.value
	}
	return f.InitialValue
}

func (f *Field) clean(data string) (string, error) {
	isMultichoice := f.MultiValue || (f.Choicer != nil && f.Choicer.IsMultivalue())

	if f.Required && data == "" {
		f.Error = FieldErrorf(ErrRequired, "%s is required", f.Label)
		return "", f.Error
	}

	var values []string
	if isMultichoice {
		values = strings.Split(data, ",")
	} else {
		values = []string{data}
	}

	if f.MaxItems != nil && len(values) > *f.MaxItems {
		f.Error = FieldErrorf(ErrOutOfRange, "%s exceeds maximum of %d items.", f.Label, *f.MaxItems)
		return "", f.Error
	}

	for _, v := range values {
		if f.MaxLength > 0 && len(v) > f.MaxLength {
			f.Error = FieldErrorf(ErrOutOfRange, "%s exceeds maximum length of %d characters.", f.Label, f.MaxLength)
			return "", f.Error
		}

		if f.MinLength > 0 && len(v) < f.MinLength {
			f.Error = FieldErrorf(ErrOutOfRange, "%s is less than minimum length of %d characters.", f.Label, f.MinLength)
			return "", f.Error
		}

		if f.Pattern != "" {
			r := regexp.MustCompile(f.Pattern)
			if !r.MatchString(v) {
				msg := f.PatternInvalidMsg
				if msg == "" {
					msg = fmt.Sprintf("%s does not match pattern '%s'", f.Label, f.Pattern)
				}
				f.Error = FieldErrorf(ErrInvalid, "%s", msg)
				return "", f.Error
			}
		}

		if f.InputType == input.TypeNumber && (f.Required || v != "") {
			ndata, err := strconv.Atoi(v)
			if err != nil {
				f.Error = FieldErrorf(ErrInvalid, "%s is not a valid number.", f.Name)
				return "", f.Error
			}

			if f.Min != nil && ndata < *f.Min {
				f.Error = FieldErrorf(ErrOutOfRange, "%s is less than minimum value of %d.", f.Label, *f.Min)
				return "", f.Error
			}
			if f.Max != nil && ndata > *f.Max {
				f.Error = FieldErrorf(ErrOutOfRange, "%s exceeds maximum value of %d.", f.Label, *f.Max)
				return "", f.Error
			}
		}

		if f.Choicer != nil && (f.Required || v != "") {
			found := false
			for _, choice := range f.Choicer.List() {
				if choice.Value == v {
					found = true
					break
				}
			}
			if !found {
				f.Error = FieldErrorf(ErrInvalid, "'%s' is not a valid choice for %s.", f.Value(), f.Label)
				return "", f.Error
			}
		}
	}

	return data, nil
}

// nolint: gocyclo
func WidgetProps[T any](f *Field) T {
	props := new(T)

	propsValue := reflect.ValueOf(props).Elem()
	propsType := propsValue.Type()

	for i := range propsType.NumField() {
		structField := propsType.Field(i)
		propsFieldValue := propsValue.FieldByName(structField.Name)
		if propsFieldValue.CanSet() {
			switch {
			case structField.Name == "ID" && structField.Type.Kind() == reflect.String:
				propsFieldValue.SetString(f.ID())
			case structField.Name == "Name" && structField.Type.Kind() == reflect.String:
				propsFieldValue.SetString(f.Name)
			case structField.Name == "Type" && structField.Type.Kind() == reflect.String:
				propsFieldValue.SetString(string(f.InputType))
			case structField.Name == "Placeholder" && structField.Type.Kind() == reflect.String:
				propsFieldValue.SetString(f.Placeholder)
			case structField.Name == "Value" && structField.Type.Kind() == reflect.String:
				propsFieldValue.SetString(f.Value())
			case structField.Name == "Checked" && structField.Type.Kind() == reflect.Bool:
				propsFieldValue.SetBool(f.Checked)
			case structField.Name == "Disabled" && structField.Type.Kind() == reflect.Bool:
				propsFieldValue.SetBool(f.Disabled)
			case structField.Name == "Readonly" && structField.Type.Kind() == reflect.Bool:
				propsFieldValue.SetBool(f.Readonly)
			case structField.Name == "Required" && structField.Type.Kind() == reflect.Bool:
				propsFieldValue.SetBool(f.Required)
			case structField.Name == "HasError" && structField.Type.Kind() == reflect.Bool:
				propsFieldValue.SetBool(f.Error != nil)
			case structField.Name == "MaxLength" && structField.Type.Kind() == reflect.Int:
				propsFieldValue.SetInt(int64(f.MaxLength))
			case structField.Name == "MinLength" && structField.Type.Kind() == reflect.Int:
				propsFieldValue.SetInt(int64(f.MinLength))
			case f.Min != nil && structField.Name == "Min" && structField.Type.Kind() == reflect.Int:
				propsFieldValue.SetInt(int64(*f.Min))
			case f.Max != nil && structField.Name == "Max" && structField.Type.Kind() == reflect.Int:
				propsFieldValue.SetInt(int64(*f.Max))
			case f.Max != nil && structField.Name == "Step" && structField.Type.Kind() == reflect.Int:
				propsFieldValue.Addr().SetInt(int64(*f.Step))
			case structField.Name == "Pattern" && structField.Type.Kind() == reflect.String:
				propsFieldValue.SetString(f.Pattern)
			case structField.Name == "Attributes" && f.Pattern != "" && structField.Type.Kind() == reflect.TypeOf(templ.Attributes{}).Kind():
				if propsFieldValue.IsNil() {
					propsFieldValue.Set(reflect.ValueOf(templ.Attributes{}))
				}
				propsFieldValue.SetMapIndex(reflect.ValueOf("pattern"), reflect.ValueOf(f.Pattern))
			case structField.Name == "Rows" && structField.Type.Kind() == reflect.Int:
				propsFieldValue.SetInt(int64(f.Rows))
			case structField.Name == "AutoResize" && structField.Type.Kind() == reflect.Bool:
				propsFieldValue.SetBool(f.AutoResize)
			}
		}
	}

	return *props
}

func (f *Field) IsValid() bool {
	if f == nil {
		return true
	}
	return f.Error == nil
}

func (f *Field) IsHidden() bool {
	if f == nil {
		return true
	}
	return f.Hidden
}

func (f *Field) ID() string {
	if f == nil {
		return ""
	}
	if f.id == "" {
		return f.Name
	}
	return f.id
}

func (f *Field) Widget(props ...HTMLBaseProps) templ.Component {
	p := HTMLBaseProps{}
	if len(props) > 0 {
		p = props[0]
	}
	p.Attributes = mergeAttributes(f.WidgetProps.Attributes, p.Attributes)
	p.Class = utils.TwMerge(f.WidgetProps.Class, p.Class)

	f.id = f.Name
	if f.form != nil {
		f.id = f.form.IDPrefix + f.Name
	}

	widget := f.WidgetComponent

	if widget == nil && f.Hidden {
		widget = hiddenWidget
	}

	if widget == nil {
		if f.Choicer != nil && !f.Choicer.IsMultivalue() {
			widget = selectWidget
		} else if f.Choicer != nil && f.Choicer.IsMultivalue() {
			widget = MultiSelectWidget
		} else {
			widget = inputWidget
		}
	}

	f.WidgetComponent = widget
	return widget(f, p)
}

func (f *Field) Component(props ...HTMLBaseProps) templ.Component {
	f.componentDepth++

	// Avoid re-rendering with itself. This can happen to fields, because they are form items as well.
	if f.componentDepth > 1 {
		return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			return templ.GetChildren(ctx).Render(ctx, w)
		})
	}

	p := HTMLBaseProps{}
	if len(props) > 0 {
		p = props[0]
	}

	defaultFormItem := StackedFormItem
	if f.form != nil && f.form.DefaultFormItem != nil {
		defaultFormItem = f.form.DefaultFormItem
	}

	c := f.FormItem
	if c == nil {
		if f.Hidden {
			c = hiddenWidgetFormItem
		} else {
			c = defaultFormItem
		}
	}
	return c(f, p)
}

type FieldList []*Field

func (f FieldList) ByName(name string) *Field {
	for _, field := range f {
		if field.Name == name {
			return field
		}
	}
	return nil
}

var _ FormItem = &Field{}

func (f *Field) FieldsList() FieldList {
	return FieldList{f}
}

type FormItem interface {
	Component(...HTMLBaseProps) templ.Component
	FieldsList() FieldList
}

type SectionComponent func(*Section, ...HTMLBaseProps) templ.Component

type Section struct {
	Title  string
	Fields FieldList

	Section SectionComponent
}

var _ FormItem = &Section{}

func (s *Section) FieldsList() FieldList {
	return s.Fields
}

func (s *Section) Component(props ...HTMLBaseProps) templ.Component {
	p := HTMLBaseProps{}
	if len(props) > 0 {
		p = props[0]
	}

	c := s.Section
	if c == nil {
		c = defaultSection
	}
	return c(s, p)
}

type formError struct {
	fieldErrors    map[string]error
	nonFieldErrors []error
}

func (f *formError) Error() string {
	return "Please provide valid input"
}

func (f *formError) addFieldError(name string, err error) {
	if f.fieldErrors == nil {
		f.fieldErrors = make(map[string]error)
	}
	f.fieldErrors[name] = err
}

func (f *formError) addNonFieldError(err error) {
	f.nonFieldErrors = append(f.nonFieldErrors, err)
}

func (f *formError) Unwrap() []error {
	errors := f.nonFieldErrors
	for _, err := range f.fieldErrors {
		errors = append(errors, err)
	}
	return errors
}

type FormAction string

var (
	Submit FormAction = ""
)

type Form struct {
	IDPrefix string
	Fields   []FormItem
	Actions  []FormAction

	Clean func(map[string]string) (map[string]string, error)

	Error *formError

	DefaultFormItem FieldComponent

	cleanedData map[string]string
}

func (f *Form) fields() FieldList {
	fields := make(FieldList, 0)
	for _, item := range f.Fields {
		fields = append(fields, item.FieldsList()...)
	}
	return fields
}

func (f *Form) AddFieldError(name string, err error) {
	for _, field := range f.fields() {
		if field.Name == name {
			field.Error = err
			if f.Error == nil {
				f.Error = &formError{}
			}
			f.Error.addFieldError(name, err)
		}
	}
}

func (f *Form) AddNonFieldError(err error) {
	if f.Error == nil {
		f.Error = &formError{}
	}
	f.Error.addNonFieldError(err)
}

func (f *Form) HasErrors() bool {
	return f.Error != nil && (len(f.Error.fieldErrors) > 0 || len(f.Error.nonFieldErrors) > 0)
}

func (f *Form) Validate(data url.Values) error {
	var err error
	f.cleanedData = make(map[string]string)

	for _, field := range f.fields() {
		value := data.Get(field.Name)
		field.SetValue(value)

		value, err = field.clean(value)
		if err != nil {
			f.AddFieldError(field.Name, err)
		} else {
			f.cleanedData[field.Name] = value
		}
	}

	// If there are field errors, bail before cleaning the entire dataset
	if f.Error != nil {
		return f.Error
	}

	if f.Clean != nil {
		cleanedData, err := f.Clean(f.cleanedData)
		if err != nil {
			f.AddNonFieldError(err)
			return f.Error
		}
		f.cleanedData = cleanedData
	}

	return nil
}

func (f *Form) GetCleanedData() map[string]string {
	return f.cleanedData
}

func (f *Form) GetNonFieldErrors() []error {
	if f.Error == nil {
		return nil
	}
	return f.Error.nonFieldErrors
}

func (f *Form) Render(ctx context.Context, w io.Writer) error {
	for _, field := range f.fields() {
		field.form = f
	}
	return f.form().Render(ctx, w)
}

func Error(msg string, args ...any) error {
	return fmt.Errorf(msg, args...)
}
