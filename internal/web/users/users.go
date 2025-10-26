// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package users

import (
	"net/http"

	"github.com/a-h/templ"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ReviewSignal/orderly-ape/internal/dex"
	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
	"github.com/ReviewSignal/orderly-ape/internal/web/entity"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/table"
)

var _ datatable.DataTabler[entity.User] = &UsersController{}
var _ datatable.DataLister[entity.User] = &UsersController{}
var _ datatable.DataCreator[entity.User] = &UsersController{}
var _ datatable.DataEditor[entity.User] = &UsersController{}
var _ datatable.DataDeleter[entity.User] = &UsersController{}

type UsersController struct {
	client.Client
	DexClient           *dex.Client
	ControllerNamespace string
}

func (c *UsersController) ViewSet() *datatable.ViewSet[entity.User] {
	return datatable.NewViewSet[entity.User](c, "users", datatable.ViewSetOptions{
		InlineCreate: true,
		InlineEdit:   true,
	})
}

func (c *UsersController) GetColumns() datatable.Columns {
	return []datatable.Column{
		{
			Name: "User",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return UserName(o), table.CellProps{}
			},
		},
	}
}

func (c *UsersController) ListObjects(r *http.Request) ([]entity.User, *datatable.DataPaginator, error) {
	return c.listDexUsers(r)
}

func (c *UsersController) CreateForm(r *http.Request, p datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	return c.createDexUser(r, p)
}

func (c *UsersController) EditForm(r *http.Request, p datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	return c.editDexUser(r, p)
}

func (c *UsersController) DeleteObjectByName(r *http.Request, key client.ObjectKey) error {
	return c.deleteDexUser(r, key)
}
