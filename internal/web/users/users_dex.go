// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package users

import (
	"net/http"
	gourl "net/url"
	"slices"
	"strings"

	dexapi "github.com/dexidp/dex/api/v2"
	"golang.org/x/crypto/bcrypt"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
	"github.com/ReviewSignal/orderly-ape/internal/web/entity"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/input"
	"github.com/ReviewSignal/orderly-ape/web/utils"
)

// listDexUsers returns the list of users from DEX
func (c *UsersController) listDexUsers(r *http.Request) ([]entity.User, *datatable.DataPaginator, error) {
	resp, err := c.DexClient.ListPasswords(r.Context(), &dexapi.ListPasswordReq{})
	if err != nil {
		return nil, nil, err
	}

	filtered := make([]entity.User, 0, len(resp.Passwords))
	search := strings.ToLower(datatable.SearchTerm(r))

	for _, password := range resp.Passwords {
		if strings.Contains(strings.ToLower(password.Email), search) ||
			strings.Contains(strings.ToLower(password.Username), search) {
			// Create user from password and store pointer to avoid loop variable capture
			u := entity.WrapDexPassword(password)
			filtered = append(filtered, u)
		}
	}

	slices.SortStableFunc(filtered, func(a, b entity.User) int {
		emailA := a.GetEmail()
		emailB := b.GetEmail()
		return strings.Compare(emailA, emailB)
	})

	usersIter, totalPages, currentPage, _ := utils.PaginateRequest(r, filtered, datatable.ItemsPerPage)
	users := slices.Collect(usersIter)

	return users, &datatable.DataPaginator{TotalPages: totalPages, CurrentPage: currentPage}, nil
}

// CreateForm returns the form for creating a DEX user
func (c *UsersController) createDexUser(r *http.Request, _ datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Field{Name: "email", Label: "Email", Required: true, Placeholder: "user@example.com", Pattern: "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"},
			&forms.Field{Name: "username", Label: "Username", Required: true, Placeholder: "johndoe"},
			&forms.Field{Name: "password", Label: "Password", Required: true, Placeholder: "Enter password", InputType: input.TypePassword},
		},
	}

	return form, func() (*gourl.URL, error) {
		data := form.GetCleanedData()

		// Hash the password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(data["password"]), bcrypt.DefaultCost)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		// All users are admins - use email as user_id since role is not needed
		resp, err := c.DexClient.CreatePassword(r.Context(), &dexapi.CreatePasswordReq{
			Password: &dexapi.Password{
				Email:    data["email"],
				Hash:     hashedPassword,
				Username: data["username"],
				UserId:   data["email"],
			},
		})
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		if resp.AlreadyExists {
			form.AddNonFieldError(forms.Error("User with this email already exists"))
			return nil, forms.Error("User with this email already exists")
		}

		return nil, nil
	}, nil
}

// EditForm returns the form for editing a DEX user
func (c *UsersController) editDexUser(r *http.Request, p datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	user, ok := p.(entity.User)
	if !ok {
		return nil, nil, forms.Error("Invalid user entry")
	}

	email := user.GetEmail()
	username := user.GetUsername()

	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Field{Name: "email", Label: "Email", Required: true, Readonly: true, InitialValue: email},
			&forms.Field{Name: "username", Label: "Username", Required: true, Placeholder: "johndoe", InitialValue: username},
			&forms.Field{Name: "password", Label: "New Password (leave blank to keep current)", Placeholder: "Enter new password", InputType: input.TypePassword},
		},
	}

	return form, func() (*gourl.URL, error) {
		data := form.GetCleanedData()

		// Prepare update request
		updateReq := &dexapi.UpdatePasswordReq{
			Email:       email,
			NewUsername: data["username"],
		}

		// Only update password if provided
		if data["password"] != "" {
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(data["password"]), bcrypt.DefaultCost)
			if err != nil {
				form.AddNonFieldError(err)
				return nil, err
			}
			updateReq.NewHash = hashedPassword
		}

		resp, err := c.DexClient.UpdatePassword(r.Context(), updateReq)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		if resp.NotFound {
			form.AddNonFieldError(forms.Error("User not found"))
			return nil, forms.Error("User not found")
		}

		return nil, nil
	}, nil
}

// DeleteObjectByName deletes a DEX user
func (c *UsersController) deleteDexUser(r *http.Request, key client.ObjectKey) error {
	// The key.Name for DEX users is the email
	email := key.Name

	resp, err := c.DexClient.DeletePassword(r.Context(), &dexapi.DeletePasswordReq{
		Email: email,
	})
	if err != nil {
		return err
	}

	if resp.NotFound {
		return forms.Error("User not found")
	}

	return nil
}
