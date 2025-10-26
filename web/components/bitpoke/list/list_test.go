// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package list_test

import (
	"context"
	"io"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PuerkitoBio/goquery"

	. "github.com/ReviewSignal/orderly-ape/web/components/bitpoke/list"
	. "github.com/ReviewSignal/orderly-ape/web/components/bitpoke/list/internal/test"
)

func TestEditableList(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EditableList Component Suite")
}

var _ = Describe("EditableList Component", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("SimpleEditableList", func() {
		It("should render the editable list component", func() {
			r, w := io.Pipe()
			go func() {
				_ = SimpleEditableList().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='simple-list']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the hidden input exists
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			Expect(hiddenInput.AttrOr("name", "")).To(Equal("simple-list"))

			// Check that the items container exists
			itemsContainer := wrapper.Find("[data-editable-list-items]")
			Expect(itemsContainer.Length()).To(Equal(1))

			// Check that the add button exists
			addButton := wrapper.Find("[data-editable-list-add]")
			Expect(addButton.Length()).To(Equal(1))
			Expect(addButton.Text()).To(ContainSubstring("Add Item"))

			// Check that the template exists
			template := wrapper.Find("template[data-editable-list-template]")
			Expect(template.Length()).To(Equal(1))
		})
	})

	Describe("JSONObjectsList", func() {
		It("should render the editable list with custom child component", func() {
			r, w := io.Pipe()
			go func() {
				_ = JSONObjectsList().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='json-list']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the template contains the custom input
			template := wrapper.Find("template[data-editable-list-template]")
			Expect(template.Length()).To(Equal(1))

			// Check for the name input in the template
			templateHTML, _ := template.Html()
			Expect(templateHTML).To(ContainSubstring("name=\"name\""))
			Expect(templateHTML).To(ContainSubstring("placeholder=\"Enter name\""))
		})
	})

	Describe("ComplexJSONObjectsList", func() {
		It("should render the editable list with multiple inputs", func() {
			r, w := io.Pipe()
			go func() {
				_ = ComplexJSONObjectsList().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='complex-list']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the template contains both input and checkbox
			template := wrapper.Find("template[data-editable-list-template]")
			Expect(template.Length()).To(Equal(1))

			templateHTML, _ := template.Html()
			Expect(templateHTML).To(ContainSubstring("name=\"name\""))
			Expect(templateHTML).To(ContainSubstring("name=\"agreements\""))
			Expect(templateHTML).To(ContainSubstring("Accept terms and conditions"))
		})
	})

	Describe("ComplexJSONObject", func() {
		It("should render the editable list with name input and other inputs", func() {
			r, w := io.Pipe()
			go func() {
				_ = ComplexJSONObject().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='keyed-list']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the template contains input without name (name input) and checkbox with name
			template := wrapper.Find("template[data-editable-list-template]")
			Expect(template.Length()).To(Equal(1))

			templateHTML, _ := template.Html()
			Expect(templateHTML).To(ContainSubstring("placeholder=\"Enter key\""))
			Expect(templateHTML).To(ContainSubstring("name=\"agreements\""))
		})
	})

	Describe("DisabledEditableList", func() {
		It("should render the editable list as disabled", func() {
			r, w := io.Pipe()
			go func() {
				_ = DisabledEditableList().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper has disabled attribute
			wrapper := doc.Find("[data-editable-list-name='disabled-list']")
			Expect(wrapper.Length()).To(Equal(1))
			Expect(wrapper.AttrOr("data-editable-list-disabled", "")).To(Equal("true"))

			// Check that the add button does not exist (disabled lists shouldn't have add button)
			addButton := wrapper.Find("[data-editable-list-add]")
			Expect(addButton.Length()).To(Equal(0))
		})
	})

	Describe("RequiredEditableList", func() {
		It("should render the editable list as required", func() {
			r, w := io.Pipe()
			go func() {
				_ = RequiredEditableList().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper has required attribute
			wrapper := doc.Find("[data-editable-list-name='required-list']")
			Expect(wrapper.Length()).To(Equal(1))
			Expect(wrapper.AttrOr("data-editable-list-required", "")).To(Equal("true"))

			// Check that the hidden input has required attribute
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			_, exists := hiddenInput.Attr("required")
			Expect(exists).To(BeTrue())
		})
	})

	Describe("ListItem Component", func() {
		It("should render list item with children", func() {
			r, w := io.Pipe()
			go func() {
				_ = ListItem(ListItemProps{}).Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the list item exists
			item := doc.Find("[data-editable-list-item]")
			Expect(item.Length()).To(Equal(1))

			// Check that the delete button exists
			deleteButton := item.Find("[data-editable-list-item-delete]")
			Expect(deleteButton.Length()).To(Equal(1))

			// Check that the content container exists
			contentContainer := item.Find("[data-editable-list-item-content]")
			Expect(contentContainer.Length()).To(Equal(1))
		})
	})

	Describe("EditableListWithSimpleValue", func() {
		It("should render the editable list with initial simple list value", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithSimpleValue().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-value']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the hidden input has the initial value
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			Expect(hiddenInput.AttrOr("value", "")).To(Equal(`["item1", "item2", "item3"]`))
		})
	})

	Describe("EditableListWithObjectValue", func() {
		It("should render the editable list with initial object list value", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithObjectValue().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-objects']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the hidden input has the initial value
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			Expect(hiddenInput.AttrOr("value", "")).To(Equal(`[{"name": "Alice"}, {"name": "Bob"}]`))
		})
	})

	Describe("EditableListWithKeyedValue", func() {
		It("should render the editable list with initial keyed object value", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithKeyedValue().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-keyed']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the hidden input has the initial value
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			expectedValue := `{"key1": {"active": true}, "key2": {"active": false}}`
			Expect(hiddenInput.AttrOr("value", "")).To(Equal(expectedValue))
		})
	})

	Describe("EditableListWithSelect", func() {
		It("should render the editable list with select inputs", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithSelect().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-select']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the template contains select element
			template := wrapper.Find("template[data-editable-list-template]")
			Expect(template.Length()).To(Equal(1))

			templateHTML, _ := template.Html()
			Expect(templateHTML).To(ContainSubstring("select"))
			Expect(templateHTML).To(ContainSubstring("name=\"color\""))
		})
	})

	Describe("EditableListWithSelectValue", func() {
		It("should render the editable list with select and initial values", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithSelectValue().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-select-value']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the hidden input has the initial value
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			expectedValue := `[{"name": "Alice", "color": "red"}, {"name": "Bob", "color": "blue"}]`
			Expect(hiddenInput.AttrOr("value", "")).To(Equal(expectedValue))
		})
	})

	Describe("EditableListWithMultiSelect", func() {
		It("should render the editable list with multi-select inputs", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithMultiSelect().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-multiselect']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the template contains multi-select element
			template := wrapper.Find("template[data-editable-list-template]")
			Expect(template.Length()).To(Equal(1))

			templateHTML, _ := template.Html()
			Expect(templateHTML).To(ContainSubstring("select"))
			Expect(templateHTML).To(ContainSubstring("multiple"))
			Expect(templateHTML).To(ContainSubstring("name=\"tags\""))
		})
	})

	Describe("EditableListWithMultiSelectValue", func() {
		It("should render the editable list with multi-select and initial values", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithMultiSelectValue().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-multiselect-value']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the hidden input has the initial value
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			expectedValue := `[{"name": "Alice", "tags": ["tag1", "tag2"]}, {"name": "Bob", "tags": ["tag3"]}]`
			Expect(hiddenInput.AttrOr("value", "")).To(Equal(expectedValue))
		})
	})

	Describe("EditableListWithMaxItems", func() {
		It("should render the editable list with MaxItems prop", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithMaxItems().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-max']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the max-items attribute is set
			Expect(wrapper.AttrOr("data-editable-list-max-items", "")).To(Equal("3"))
		})
	})

	Describe("EditableListWithExtraItems", func() {
		It("should render the editable list with ExtraItems prop", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithExtraItems().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-extra']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the extra-items attribute is set
			Expect(wrapper.AttrOr("data-editable-list-extra-items", "")).To(Equal("2"))
		})
	})

	Describe("EditableListWithNoExtraItems", func() {
		It("should render the editable list with ExtraItems=0", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithNoExtraItems().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-no-extra']")
			Expect(wrapper.Length()).To(Equal(1))

			// When ExtraItems=0, the attribute should not be present
			_, exists := wrapper.Attr("data-editable-list-extra-items")
			Expect(exists).To(BeFalse())
		})
	})

	Describe("EditableListWithMaxAndExtra", func() {
		It("should render the editable list with both MaxItems and ExtraItems props", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListWithMaxAndExtra().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-with-both']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that both attributes are set
			Expect(wrapper.AttrOr("data-editable-list-max-items", "")).To(Equal("5"))
			Expect(wrapper.AttrOr("data-editable-list-extra-items", "")).To(Equal("3"))
		})
	})

	Describe("EditableListReadOnly", func() {
		It("should render the editable list with ReadOnly prop", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListReadOnly().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-readonly']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the readonly attribute is set
			Expect(wrapper.AttrOr("data-editable-list-readonly", "")).To(Equal("true"))

			// Check that the add button does not exist (readonly lists shouldn't have add button)
			addButton := wrapper.Find("[data-editable-list-add]")
			Expect(addButton.Length()).To(Equal(0))
		})
	})

	Describe("EditableListReadOnlyComplex", func() {
		It("should render the editable list with ReadOnly prop and complex data", func() {
			r, w := io.Pipe()
			go func() {
				_ = EditableListReadOnlyComplex().Render(ctx, w)
				_ = w.Close()
			}()
			doc, err := goquery.NewDocumentFromReader(r)
			Expect(err).NotTo(HaveOccurred())

			// Check that the editable list wrapper exists
			wrapper := doc.Find("[data-editable-list-name='list-readonly-complex']")
			Expect(wrapper.Length()).To(Equal(1))

			// Check that the readonly attribute is set
			Expect(wrapper.AttrOr("data-editable-list-readonly", "")).To(Equal("true"))

			// Check that the add button does not exist
			addButton := wrapper.Find("[data-editable-list-add]")
			Expect(addButton.Length()).To(Equal(0))

			// Check that the initial value is set
			hiddenInput := wrapper.Find("input[type='hidden'][data-editable-list-hidden-input]")
			Expect(hiddenInput.Length()).To(Equal(1))
			expectedValue := `[{"name": "Alice", "role": "admin"}, {"name": "Bob", "role": "user"}]`
			Expect(hiddenInput.AttrOr("value", "")).To(Equal(expectedValue))
		})
	})
})
