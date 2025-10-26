// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package datatable

import (
	"errors"
	"fmt"
	"net/http"
	gourl "net/url"
	"strings"

	"github.com/a-h/templ"
	gopluralize "github.com/gertd/go-pluralize"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ReviewSignal/orderly-ape/internal/util/common"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/alert"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/dialog"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/icon"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/table"
	"github.com/ReviewSignal/orderly-ape/web/utils"
	"github.com/ReviewSignal/orderly-ape/web/utils/url"
)

const (
	SearchParam  = "s"
	ItemsPerPage = 10
)

type Entry interface {
	client.Object
	NamespacedName() string
}

var pluralize = gopluralize.NewClient()

type FormHandlerFunc func() (*gourl.URL, error)
type FormGeneratorFunc func(*http.Request, Entry) (*forms.Form, FormHandlerFunc, error)

type DataPaginator struct {
	TotalPages    int
	CurrentPage   int
	NumberOfItems int
}

type DataCreator[T Entry] interface {
	// CreateForm returns a form for creating a new object.
	// It should return a form handling function that can be used to create the object.
	// The function should return the redirect URL.
	// For creating, the object is always nil, as it is a new object, but allows the function to be reused for editing as well.
	//
	// If there is an error in creating the form, it should return an error.

	CreateForm(*http.Request, Entry) (*forms.Form, FormHandlerFunc, error)
}

type DataEditor[T Entry] interface {
	EditForm(*http.Request, Entry) (*forms.Form, FormHandlerFunc, error)
}

type DataDeleter[T Entry] interface {
	DeleteObjectByName(*http.Request, types.NamespacedName) error
}

type DataTabler[T Entry] any

type DataLister[T Entry] interface {
	GetColumns() Columns
	ListObjects(*http.Request) ([]T, *DataPaginator, error)
}

type ViewSetOptions struct {
	InlineCreate bool
	InlineEdit   bool
}

type ObjectAction struct {
	Name        string
	Label       string
	Icon        func(...icon.Props) templ.Component
	Available   func(*http.Request, Entry) bool
	Disabled    func(*http.Request, Entry) bool
	Confirm     func(Entry, ...dialog.Props) templ.Component
	Destructive bool
	InlineForm  func(*http.Request, Entry) (*forms.Form, FormHandlerFunc, error)
	Href        func(*http.Request, Entry) string
	Target      string // e.g., "_blank" for opening in new tab
}

type Message struct {
	Variant     alert.Variant
	Title       string
	Description string

	err error
}

func NewErrorMessage(m string) *Message {
	return &Message{
		Variant:     alert.VariantDestructive,
		Title:       "Error",
		Description: m,
		err:         errors.New(m),
	}
}

func (m Message) Err() error {
	return m.err
}

type ViewSet[T Entry] struct {
	DataTabler[T]

	ListView   http.HandlerFunc
	CreateView http.HandlerFunc
	EditView   http.HandlerFunc

	ObjectActions []ObjectAction

	CreateForm FormGeneratorFunc
	EditForm   FormGeneratorFunc

	opts ViewSetOptions
	name string
}

func NewViewSet[T Entry](dt DataTabler[T], controllerName string, opts ...ViewSetOptions) *ViewSet[T] {
	vs := &ViewSet[T]{
		DataTabler:    dt,
		name:          controllerName,
		opts:          ViewSetOptions{},
		ObjectActions: []ObjectAction{},
	}

	for _, opt := range opts {
		vs.opts = opt
	}

	_, isDeleter := dt.(DataDeleter[T])

	lister, isLister := dt.(DataLister[T])
	if isLister {
		vs.ListView = vs.listView(lister)
	}

	creator, isCreator := dt.(DataCreator[T])
	if isCreator {
		vs.CreateForm = func(r *http.Request, t Entry) (*forms.Form, FormHandlerFunc, error) {
			return creator.CreateForm(r, t)
		}
		if vs.opts.InlineCreate {
			vs.CreateView = vs.ListView
		} else {
			vs.CreateView = vs.createView(creator)
		}
	}

	editor, isEditor := dt.(DataEditor[T])
	if isEditor {
		vs.EditForm = func(r *http.Request, t Entry) (*forms.Form, FormHandlerFunc, error) {
			return editor.EditForm(r, t)
		}
		if vs.opts.InlineEdit {
			vs.EditView = vs.ListView
		} else {
			vs.EditView = vs.editView(editor)
		}
	}

	if isEditor {
		editAction := ObjectAction{
			Name:  "edit",
			Label: "Edit",
			Icon:  icon.Pencil,
			Href: func(r *http.Request, t Entry) string {
				url := defaultURL(r, nil, vs.name+":edit", "name", t.GetName(), "project", t.GetNamespace())
				return url.String()
			},
		}
		if vs.opts.InlineEdit {
			editAction.Href = nil
			editAction.InlineForm = vs.EditForm
		}
		vs.ObjectActions = append(vs.ObjectActions, editAction)
	}

	if isDeleter {
		vs.ObjectActions = append(vs.ObjectActions, ObjectAction{
			Name:        "delete",
			Label:       "Delete",
			Icon:        icon.X,
			Destructive: true,
			Confirm:     ConfirmDelete,
		})
	}

	return vs
}

type props struct {
	r *http.Request
}

type ActionProps struct {
	Name        string
	Label       string
	Icon        func(...icon.Props) templ.Component
	Disabled    bool
	Confirm     func(Entry, ...dialog.Props) templ.Component
	Destructive bool
	InlineForm  *forms.Form
	Handler     FormHandlerFunc
	Href        string
	Target      string
	ObjectName  string
}

type ListItemProps[T Entry] struct {
	Object  T
	Actions []ActionProps
}

type ListProps[T Entry] struct {
	props
	Columns    Columns
	Items      []ListItemProps[T]
	Paginator  *DataPaginator
	HasActions bool

	InlineCreateTitle       string
	InlineCreateDescription string
	InlineCreateForm        *forms.Form

	Message *Message

	Title     string
	CreateURL string
}

func SearchTerm(r *http.Request) string {
	return r.URL.Query().Get(SearchParam)
}

func (p ListProps[T]) SearchTerm() string {
	return SearchTerm(p.r)
}

func VerboseName(obj any) string {
	name := fmt.Sprintf("%T", obj)
	if namer, ok := obj.(interface{ VerboseName() string }); ok {
		name = namer.VerboseName()
	}
	return name
}

var DisplayName = common.DisplayName

// nolint:gocyclo
func (c *ViewSet[T]) listView(lister DataLister[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		var message *Message

		l := log.FromContext(r.Context()).WithValues("controller", c.name+"-datatable", "action", "list")

		var obj T
		var form *forms.Form
		var formHandler FormHandlerFunc

		objects, paginator, err := lister.ListObjects(r)
		if err != nil {
			l.Error(err, "Failed to list objects")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		createURL, _ := url.RouterURL(url.Router(r.Context()), c.name+":create")
		verboseName := VerboseName(obj)
		verbosePluralName := pluralize.Plural(verboseName)

		items := make([]ListItemProps[T], len(objects))
		for i, obj := range objects {
			readOnlyInterfacer, hasReadOnly := any(obj).(interface{ IsReadOnly() bool })
			isReadOnly := hasReadOnly && readOnlyInterfacer.IsReadOnly()

			actions := make([]ActionProps, 0, len(c.ObjectActions))
			for _, action := range c.ObjectActions {
				if isReadOnly && action.Destructive {
					continue // Skip destructive actions for read-only objects
				}

				if isReadOnly && action.Name == "edit" {
					continue // Skip edit action for read-only objects
				}

				if action.Available != nil && !action.Available(r, obj) {
					continue
				}

				disabled := false
				if action.Disabled != nil {
					disabled = action.Disabled(r, obj)
				}

				actionProps := ActionProps{
					Name:        action.Name,
					Label:       action.Label,
					Icon:        action.Icon,
					Destructive: action.Destructive,
					Target:      action.Target,
					Disabled:    disabled,
				}
				if action.Href != nil {
					actionProps.Href = action.Href(r, obj)
				} else if action.InlineForm != nil {
					form, formHandler, err = action.InlineForm(r, obj)
					if err != nil {
						l.Error(err, "Failed to create inline form")
						message = NewErrorMessage("Internal Server Error")
						return
					}
					actionProps.InlineForm = form
					actionProps.Handler = formHandler
				} else if action.Confirm != nil {
					actionProps.Confirm = action.Confirm
				}
				actions = append(actions, actionProps)
			}
			items[i] = ListItemProps[T]{
				Object:  obj,
				Actions: actions,
			}
		}

		var createForm *forms.Form
		var createFormHandler FormHandlerFunc

		defer func() {
			ctx := utils.SetPageTitle(r.Context(), verbosePluralName)
			err := ListView(ListProps[T]{
				props:                   props{r: r},
				Title:                   verbosePluralName,
				Columns:                 lister.GetColumns(),
				Items:                   items,
				HasActions:              len(c.ObjectActions) > 0,
				Paginator:               paginator,
				CreateURL:               createURL,
				InlineCreateDescription: fmt.Sprintf("Create a new %s", strings.ToLower(verboseName)),
				InlineCreateTitle:       fmt.Sprintf("Create %s", verboseName),
				InlineCreateForm:        createForm,
				Message:                 message,
			}).Render(ctx, w)
			if err != nil {
				l.Error(err, "failed to render create view")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		if c.opts.InlineCreate && c.CreateForm != nil {
			createForm, createFormHandler, err = c.CreateForm(r, obj)
			if err != nil {
				l.Error(err, "Failed to create inline form")
				w.WriteHeader(http.StatusInternalServerError)
				message = NewErrorMessage("Internal Server Error")
				return
			}
		}

		// Handle POST requests for creating or deleting objects

		if r.Method != http.MethodPost {
			return
		}

		if err = r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			message = NewErrorMessage("Please provide valid form data")
			return
		}

		if r.Form.Get("action") == "delete" {
			deleter, ok := lister.(DataDeleter[T])
			if !ok {
				l.Error(fmt.Errorf("data lister does not implement DataDeleter interface"), "Cannot delete object")
				message = NewErrorMessage("Internal error")
				return
			}
			objName := r.Form.Get("object")
			if objName == "" {
				l.Error(fmt.Errorf("name parameter is missing"), "Cannot delete object")
				message = NewErrorMessage("Missing name parameter")
				return
			}

			parts := strings.SplitN(objName, "/", 2)
			if len(parts) != 2 {
				l.Error(fmt.Errorf("invalid name format: %s", objName), "Cannot delete object")
				message = NewErrorMessage("Invalid name format")
				return
			}
			name := types.NamespacedName{
				Namespace: parts[0],
				Name:      parts[1],
			}

			err := deleter.DeleteObjectByName(r, name)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				message = NewErrorMessage(err.Error())
				return
			}

			http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
			return
		}

		if r.Form.Get("action") == "create" && createFormHandler != nil {
			if err = createForm.Validate(r.Form); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			var next *gourl.URL
			if next, err = createFormHandler(); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			nextURL := defaultURL(r, next, c.name+":list").String()

			http.Redirect(w, r, nextURL, http.StatusSeeOther)
			return
		}

		if r.Form.Get("action") != "" {
			// Clear the create form when handling non-create actions to prevent
			// the create drawer from opening with edit data when there are errors
			form = nil
			formHandler = nil

			var item ListItemProps[T]
			for _, it := range items {
				if it.Object.NamespacedName() == r.Form.Get("object") {
					item = it
					break
				}
			}

			for _, action := range item.Actions {
				if action.Name == r.Form.Get("action") {
					form = action.InlineForm
					formHandler = action.Handler
					break
				}
			}

			if formHandler == nil {
				l.Error(fmt.Errorf("no handler for action '%s'", r.Form.Get("action")), "Cannot handle action")
				message = NewErrorMessage("Internal error")
				return
			}

			if form != nil {
				if err = form.Validate(r.Form); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
			}

			var next *gourl.URL
			if next, err = formHandler(); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				message = NewErrorMessage(err.Error())
				return
			}
			nextURL := defaultURL(r, next, c.name+":list").String()

			http.Redirect(w, r, nextURL, http.StatusSeeOther)
		}
	}
}

type CreateProps[T Entry] struct {
	props
	Title string
	Form  *forms.Form
}

func (c *ViewSet[T]) createView(creator DataCreator[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := log.FromContext(r.Context()).WithValues("controller", c.name+"-datatable", "action", "create")
		var obj T

		verboseName := VerboseName(obj)
		// verbosePluralName := pluralize.Plural(verboseName)

		form, formHandler, err := creator.CreateForm(r, obj)
		if err != nil {
			l.Error(err, "Failed to create form")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if form == nil {
			l.Error(fmt.Errorf("form is nil"), "Failed to create form")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer func() {
			ctx := utils.SetPageTitle(r.Context(), "Create "+verboseName)
			err := CreateView(CreateProps[T]{
				props: props{r: r},
				Title: "Create " + VerboseName(obj),
				Form:  form,
			}).Render(ctx, w)
			if err != nil {
				l.Error(err, "failed to render create view")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		if r.Method == http.MethodPost {
			var err error

			if err = r.ParseForm(); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				form.AddNonFieldError(forms.Error("Please provide valid form data"))
				return
			}
			if err = form.Validate(r.Form); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			var next *gourl.URL
			if next, err = formHandler(); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			nextURL := defaultURL(r, next, c.name+":list").String()

			http.Redirect(w, r, nextURL, http.StatusSeeOther)
		}
	}
}

type EditProps[T Entry] struct {
	props
	Title string
	Form  *forms.Form
}

func (c *ViewSet[T]) editView(editor DataEditor[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := log.FromContext(r.Context()).WithValues("controller", c.name+"-datatable", "action", "edit")
		var obj T

		verboseName := VerboseName(obj)
		// verbosePluralName := pluralize.Plural(verboseName)

		form, formHandler, err := editor.EditForm(r, obj)
		if err != nil || form == nil {
			l.Error(err, "Failed to create edit form")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer func() {
			ctx := utils.SetPageTitle(r.Context(), "Edit "+verboseName)
			err := EditView(EditProps[T]{
				props: props{r: r},
				Title: "Edit " + verboseName,
				Form:  form,
			}).Render(ctx, w)
			if err != nil {
				l.Error(err, "failed to render edit view")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		if r.Method == http.MethodPost {
			var err error

			if err = r.ParseForm(); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				form.AddNonFieldError(forms.Error("Please provide valid form data"))
				return
			}
			if err = form.Validate(r.Form); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			var next *gourl.URL
			if next, err = formHandler(); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			nextURL := defaultURL(r, next, c.name+":list").String()

			http.Redirect(w, r, nextURL, http.StatusSeeOther)
		}
	}
}

func (c *ViewSet[T]) RouteController(r *mux.Router) {
	if c.ListView != nil {
		r.Path("").HandlerFunc(c.ListView).Name(c.name + ":list")
	}

	if !c.opts.InlineCreate && c.CreateView != nil {
		r.Path("/create").HandlerFunc(c.CreateView).Name(c.name + ":create")
	} else if c.opts.InlineCreate && c.CreateForm != nil {
		r.Path("").Methods("POST").HandlerFunc(c.ListView).Name(c.name + ":create")
	}

	if !c.opts.InlineEdit && c.EditView != nil {
		r.Path("/{name}").HandlerFunc(c.EditView).Name(c.name + ":edit")
	} else if c.opts.InlineEdit && c.EditForm != nil {
		r.Path("").Methods("POST").HandlerFunc(c.ListView).Name(c.name + ":edit")
	}
}

type Column struct {
	Name string

	HeadProps table.HeadProps

	Field     string
	Component func(o Entry) (templ.Component, table.CellProps)
}

func LinkedObjectCell(obj Entry) (templ.Component, table.CellProps) {
	return ObjectCellLink(obj), table.CellProps{}
}

func DefaultObjectCell(obj Entry) (templ.Component, table.CellProps) {
	return ObjectCellInlineEdit(obj), table.CellProps{}
}

func (c *Column) component(obj Entry) (templ.Component, table.CellProps) {
	if c.Component != nil {
		return c.Component(obj)
	}
	return DefaultObjectCell(obj)
}

type Columns []Column

func (c Columns) GetColumns() Columns {
	return c
}

func defaultURL(r *http.Request, u *gourl.URL, routeName string, pairs ...string) *gourl.URL {
	if u != nil {
		return u
	}

	defaultURL, err := url.RouterURL(url.Router(r.Context()), routeName, pairs...)
	if err != nil {
		defaultURL = "about:invalid#reverse-" + routeName
	}

	u, err = gourl.Parse(defaultURL)
	if err != nil {
		u, _ = gourl.Parse("/#invalid-revese-" + routeName)
	}
	return u
}
