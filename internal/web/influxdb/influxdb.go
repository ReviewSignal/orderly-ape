// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package influxdb

import (
	"context"
	"net/http"
	gourl "net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ReviewSignal/orderly-ape/internal/util/common"
	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
	"github.com/ReviewSignal/orderly-ape/internal/web/entity"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/web/components/bitpoke/name"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/table"
	"github.com/ReviewSignal/orderly-ape/web/utils"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

var _ datatable.DataTabler[entity.InfluxDB] = &InfluxDBController{}
var _ datatable.DataLister[entity.InfluxDB] = &InfluxDBController{}
var _ datatable.DataCreator[entity.InfluxDB] = &InfluxDBController{}
var _ datatable.DataEditor[entity.InfluxDB] = &InfluxDBController{}
var _ datatable.DataDeleter[entity.InfluxDB] = &InfluxDBController{}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=influxdbs,verbs=get;list;watch;update;patch;create;delete

type InfluxDBController struct {
	client.Client
	Scheme              *runtime.Scheme
	ControllerNamespace string
}

func (c *InfluxDBController) ViewSet() *datatable.ViewSet[entity.InfluxDB] {
	return datatable.NewViewSet[entity.InfluxDB](c, "influxdb", datatable.ViewSetOptions{
		InlineCreate: true,
		InlineEdit:   true,
	})
}

func (c *InfluxDBController) GetColumns() datatable.Columns {
	return []datatable.Column{
		{
			Name: "Name",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return InfluxDBName(o), table.CellProps{}
			},
		},
		{
			Name: "Address",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return InfluxDBAddress(o), table.CellProps{}
			},
		},
		{
			Name: "Organization",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return InfluxDBOrganization(o), table.CellProps{}
			},
		},
	}
}

func filterInfluxDBs(influxdbs []apev1.InfluxDB, r *http.Request) entity.InfluxDBList {
	filtered := make(entity.InfluxDBList, 0, len(influxdbs))

	search := strings.ToLower(datatable.SearchTerm(r))

	for i, db := range influxdbs {
		displayName := common.DisplayName(&db)
		if strings.Contains(strings.ToLower(db.Name), search) ||
			strings.Contains(strings.ToLower(displayName), search) ||
			strings.Contains(strings.ToLower(db.Spec.Address.Value), search) ||
			strings.Contains(strings.ToLower(db.Spec.Organization.Value), search) {
			filtered = append(filtered, entity.WrapInfluxDB(&influxdbs[i]))
		}
	}

	return filtered
}

func (c *InfluxDBController) ListObjects(r *http.Request) ([]entity.InfluxDB, *datatable.DataPaginator, error) {
	list := &apev1.InfluxDBList{}
	err := c.List(r.Context(), list, client.InNamespace(c.ControllerNamespace))
	if err != nil {
		return nil, nil, err
	}

	filtered := filterInfluxDBs(list.Items, r)
	slices.SortStableFunc(filtered, func(a, b entity.InfluxDB) int {
		return strings.Compare(a.GetName(), b.GetName())
	})

	influxdbsIter, totalPages, currentPage, _ := utils.PaginateRequest(r, filtered, datatable.ItemsPerPage)
	influxdbs := slices.Collect(influxdbsIter)

	return influxdbs, &datatable.DataPaginator{TotalPages: totalPages, CurrentPage: currentPage}, nil
}

func (c *InfluxDBController) EditForm(r *http.Request, i datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	influxdb, ok := i.(entity.InfluxDB)
	if !ok {
		return nil, nil, apierrors.NewBadRequest("Invalid InfluxDB entry")
	}

	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Field{
				Name:         "displayName",
				Label:        "InfluxDB Name",
				Placeholder:  "InfluxDB display name",
				InitialValue: common.DisplayName(i),
			},
			&forms.Field{
				Name:              "name",
				Label:             "InfluxDB Resource Name",
				Description:       "The name of the InfluxDB resource in Kubernetes. This cannot be changed.",
				Required:          true,
				Placeholder:       "Enter InfluxDB name",
				MaxLength:         63,
				Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
				PatternInvalidMsg: "InfluxDB name must start with a letter and contain only lowercase letters, numbers, and hyphens.",
				InitialValue:      i.GetName(),
				Readonly:          true,
			},
			&forms.Field{
				Name:         "address",
				Label:        "Address",
				Required:     true,
				Placeholder:  "127.0.0.1:8086 or my.super.influxdb.host.com",
				InitialValue: influxdb.Spec.Address.Value,
			},
			&forms.Field{
				Name:        "token",
				Label:       "Token",
				Placeholder: "Enter InfluxDB token (leave empty to keep existing)",
				InputType:   "password",
			},
			&forms.Field{
				Name:         "organization",
				Label:        "Organization",
				Required:     true,
				Placeholder:  "Enter organization name",
				InitialValue: influxdb.Spec.Organization.Value,
				Readonly:     true,
			},
		},
	}

	return form, func() (*gourl.URL, error) {
		object := r.FormValue("object")
		name := strings.Split(object, "/")[1]

		data := form.GetCleanedData()

		// Validate name hasn't changed
		if data["name"] != name {
			err := forms.FieldErrorf(nil, "InfluxDB name cannot be changed")
			form.AddFieldError("name", err)
			return nil, err
		}

		// Validate organization hasn't changed
		if data["organization"] != influxdb.Spec.Organization.Value {
			err := forms.FieldErrorf(nil, "Organization cannot be changed")
			form.AddFieldError("organization", err)
			return nil, err
		}

		key := client.ObjectKey{
			Namespace: c.ControllerNamespace,
			Name:      name,
		}

		existing := &apev1.InfluxDB{}
		err := c.Get(r.Context(), key, existing)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		// Update annotations
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		if displayName, ok := data["displayName"]; ok && displayName != "" {
			existing.Annotations[common.DisplayNameAnnotation] = displayName
		} else {
			delete(existing.Annotations, common.DisplayNameAnnotation)
		}

		// Update address
		existing.Spec.Address = apev1.SourcedValue{
			Value: data["address"],
		}

		// Handle token secret
		tokenData := data["token"]
		if tokenData != "" {
			if existing.Spec.Token.ValueFrom == nil || existing.Spec.Token.ValueFrom.SecretKeyRef == nil {
				// Create a new secret
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: existing.Name + "-creds-",
						Namespace:    existing.Namespace,
					},
					Data: map[string][]byte{
						"token": []byte(tokenData),
					},
					Type: corev1.SecretTypeOpaque,
				}
				err := c.Create(r.Context(), secret)
				if err != nil {
					form.AddNonFieldError(err)
					return nil, err
				}

				existing.Spec.Token = apev1.SourcedValue{
					ValueFrom: &apev1.ValueFrom{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secret.Name,
							},
							Key: "token",
						},
					},
				}
			} else {
				// Update existing secret
				secret := &corev1.Secret{}
				err := c.Get(r.Context(), client.ObjectKey{
					Namespace: existing.Namespace,
					Name:      existing.Spec.Token.ValueFrom.SecretKeyRef.Name,
				}, secret)
				if err != nil {
					form.AddNonFieldError(err)
					return nil, err
				}

				secret.Data = map[string][]byte{
					"token": []byte(tokenData),
				}
				err = c.Update(r.Context(), secret)
				if err != nil {
					form.AddNonFieldError(err)
					return nil, err
				}
			}
		}

		err = c.Update(r.Context(), existing)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		return nil, nil
	}, nil
}

func (c *InfluxDBController) CreateForm(r *http.Request, i datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Section{
				Section: name.Section,
				Fields: forms.FieldList{
					&forms.Field{
						Name:        "displayName",
						Label:       "InfluxDB Name",
						Placeholder: "InfluxDB display name",
					},
					&forms.Field{
						Name:              "name",
						Label:             "InfluxDB Resource Name",
						Description:       "The name of the InfluxDB resource in Kubernetes. This cannot be changed.",
						Required:          true,
						Placeholder:       "Enter InfluxDB name",
						MaxLength:         63,
						Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
						PatternInvalidMsg: "Name must start with a letter and contain only lowercase letters, numbers, and hyphens.",
					},
				},
			},
			&forms.Field{
				Name:        "address",
				Label:       "Address",
				Required:    true,
				Placeholder: "127.0.0.1:8086 or my.super.influxdb.host.com",
			},
			&forms.Field{
				Name:        "token",
				Label:       "Token",
				Required:    true,
				Placeholder: "Enter InfluxDB token",
				InputType:   "password",
			},
			&forms.Field{
				Name:        "organization",
				Label:       "Organization",
				Required:    true,
				Placeholder: "Enter organization name",
			},
		},
	}

	return form, func() (*gourl.URL, error) {
		data := form.GetCleanedData()

		influxdb := &apev1.InfluxDB{
			ObjectMeta: metav1.ObjectMeta{
				Name:        data["name"],
				Namespace:   c.ControllerNamespace,
				Annotations: map[string]string{},
			},
			Spec: apev1.InfluxDBSpec{
				Address: apev1.SourcedValue{
					Value: data["address"],
				},
				Organization: apev1.SourcedValue{
					Value: data["organization"],
				},
			},
		}

		if displayName, ok := data["displayName"]; ok && displayName != "" {
			influxdb.Annotations[common.DisplayNameAnnotation] = displayName
		}

		// Create secret for token first
		tokenData := data["token"]
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: influxdb.Name + "-creds-",
				Namespace:    influxdb.Namespace,
			},
			Data: map[string][]byte{
				"token": []byte(tokenData),
			},
			Type: corev1.SecretTypeOpaque,
		}
		err := c.Create(r.Context(), secret)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		influxdb.Spec.Token = apev1.SourcedValue{
			ValueFrom: &apev1.ValueFrom{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: "token",
				},
			},
		}

		// Create the InfluxDB resource with the token reference
		err = c.Create(r.Context(), influxdb)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				form.AddFieldError("name", forms.FieldErrorf(err, "InfluxDB with this name already exists"))
			} else {
				form.AddNonFieldError(err)
			}
			return nil, err
		}

		// Set the InfluxDB as the owner of the secret for garbage collection
		if err := controllerutil.SetControllerReference(influxdb, secret, c.Scheme); err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		err = c.Update(r.Context(), secret)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		return nil, nil
	}, nil
}

func (c *InfluxDBController) DeleteObjectByName(r *http.Request, name client.ObjectKey) error {
	influxdb := &apev1.InfluxDB{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
	}
	err := c.Get(r.Context(), client.ObjectKeyFromObject(influxdb), influxdb)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	if !influxdb.DeletionTimestamp.IsZero() {
		return nil
	}

	// Delete associated secret if it exists
	if influxdb.Spec.Token.ValueFrom != nil && influxdb.Spec.Token.ValueFrom.SecretKeyRef != nil {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      influxdb.Spec.Token.ValueFrom.SecretKeyRef.Name,
				Namespace: influxdb.Namespace,
			},
		}
		_ = c.Delete(context.Background(), secret)
	}

	return c.Delete(r.Context(), influxdb)
}
