// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package workers

import (
	"context"
	"fmt"
	"net/http"
	gourl "net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ReviewSignal/orderly-ape/internal/util/common"
	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
	"github.com/ReviewSignal/orderly-ape/internal/web/entity"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/web/components/bitpoke/name"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/table"
	"github.com/ReviewSignal/orderly-ape/web/utils"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

var _ datatable.DataTabler[entity.Worker] = &WorkersController{}
var _ datatable.DataLister[entity.Worker] = &WorkersController{}
var _ datatable.DataCreator[entity.Worker] = &WorkersController{}
var _ datatable.DataEditor[entity.Worker] = &WorkersController{}
var _ datatable.DataDeleter[entity.Worker] = &WorkersController{}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;create;update

type WorkersController struct {
	client.Client
	ControllerNamespace string
}

func (c *WorkersController) ViewSet() *datatable.ViewSet[entity.Worker] {
	return datatable.NewViewSet[entity.Worker](c, "workers", datatable.ViewSetOptions{
		InlineCreate: true,
		InlineEdit:   true,
	})
}

func (c *WorkersController) GetColumns() datatable.Columns {
	return []datatable.Column{
		{
			Name: "Name",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return WorkerName(o), table.CellProps{}
			},
		},
		{
			Name: "Node Selector",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return WorkerNodeSelector(o), table.CellProps{}
			},
		},
		{
			Name: "Tolerations",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return WorkerTolerations(o), table.CellProps{}
			},
		},
	}
}

func filterWorkers(workers []apev1.Worker, r *http.Request) entity.WorkerList {
	filtered := make(entity.WorkerList, 0, len(workers))

	search := strings.ToLower(datatable.SearchTerm(r))

	for i, w := range workers {
		displayName := common.DisplayName(&w)
		if strings.Contains(strings.ToLower(w.Name), search) ||
			strings.Contains(strings.ToLower(displayName), search) {
			filtered = append(filtered, entity.WrapWorker(&workers[i]))
		}
	}

	return filtered
}

func (c *WorkersController) ListObjects(r *http.Request) ([]entity.Worker, *datatable.DataPaginator, error) {
	list := &apev1.WorkerList{}
	err := c.List(r.Context(), list, client.InNamespace(c.ControllerNamespace))
	if err != nil {
		return nil, nil, err
	}

	filtered := filterWorkers(list.Items, r)
	slices.SortStableFunc(filtered, func(a, b entity.Worker) int {
		return strings.Compare(a.GetName(), b.GetName())
	})

	workersIter, totalPages, currentPage, _ := utils.PaginateRequest(r, filtered, datatable.ItemsPerPage)
	workers := slices.Collect(workersIter)

	return workers, &datatable.DataPaginator{TotalPages: totalPages, CurrentPage: currentPage}, nil
}

// parseNodeSelector parses a comma-separated string into a map of node selectors
// Format: "key1=value1,key2=value2,key3" where value is optional
// Whitespace is allowed around commas but NOT around the equals sign
func parseNodeSelector(input string) map[string]string {
	result := make(map[string]string)
	if input == "" {
		return result
	}

	for pair := range strings.SplitSeq(input, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		key := parts[0]
		if key == "" {
			continue
		}

		if len(parts) == 2 {
			result[key] = parts[1]
		} else {
			result[key] = ""
		}
	}

	return result
}

// formatNodeSelector formats a map of node selectors into a comma-separated string
func formatNodeSelector(ns map[string]string) string {
	if len(ns) == 0 {
		return ""
	}

	var parts []string
	for key, value := range ns {
		if value == "" {
			parts = append(parts, key)
		} else {
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}

	slices.Sort(parts)
	return strings.Join(parts, ",")
}

// parseTolerations parses a comma-separated string into a slice of tolerations
// Format: "key1=value1:NoSchedule,key2=value2,key3" where value and effect are optional
// Whitespace is allowed around commas but NOT around equals or colon
func parseTolerations(input string) []corev1.Toleration {
	if input == "" {
		return nil
	}

	pairs := strings.Split(input, ",")
	result := make([]corev1.Toleration, 0, len(pairs))

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		toleration := corev1.Toleration{
			Operator: corev1.TolerationOpEqual,
		}

		// Split by colon to separate effect
		parts := strings.SplitN(pair, ":", 2)
		keyValue := parts[0]

		// Parse key=value or just key
		kvParts := strings.SplitN(keyValue, "=", 2)
		toleration.Key = kvParts[0]

		if toleration.Key == "" {
			continue
		}

		if len(kvParts) == 2 {
			toleration.Value = kvParts[1]
		}

		// Parse effect if present
		if len(parts) == 2 {
			effectStr := parts[1]
			if effectStr != "" {
				toleration.Effect = corev1.TaintEffect(effectStr)
			}
		}

		result = append(result, toleration)
	}

	return result
}

// formatTolerations formats a slice of tolerations into a comma-separated string
func formatTolerations(tolerations []corev1.Toleration) string {
	if len(tolerations) == 0 {
		return ""
	}

	parts := make([]string, 0, len(tolerations))
	for _, t := range tolerations {
		var part string
		if t.Value == "" {
			part = t.Key
		} else {
			part = fmt.Sprintf("%s=%s", t.Key, t.Value)
		}

		if t.Effect != "" {
			part = fmt.Sprintf("%s:%s", part, t.Effect)
		}

		parts = append(parts, part)
	}

	return strings.Join(parts, ",")
}

func (c *WorkersController) EditForm(r *http.Request, w datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	worker, ok := w.(entity.Worker)
	if !ok {
		return nil, nil, apierrors.NewBadRequest("Invalid pod placement entry")
	}

	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Field{
				Name:         "displayName",
				Label:        "Location Name",
				Placeholder:  "Location display name",
				InitialValue: common.DisplayName(w),
			},
			&forms.Field{
				Name:              "name",
				Label:             "Location Resource Name",
				Description:       "The name of Location resource in Kubernetes. This cannot be changed.",
				Required:          true,
				Placeholder:       "Enter worker name",
				MaxLength:         63,
				Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
				PatternInvalidMsg: "Worker name must start with a letter and contain only lowercase letters, numbers, and hyphens.",
				InitialValue:      w.GetName(),
				Readonly:          true,
			},
			&forms.Field{
				Name:              "namespace",
				Label:             "Namespace",
				Description:       "The namespace in the target cluster where the worker is operating. This cannot be changed.",
				Required:          true,
				Placeholder:       "Enter worker namespace (eg. orderly-ape-k6)",
				MaxLength:         63,
				Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
				PatternInvalidMsg: "Namespace must start with a letter and contain only lowercase letters, numbers, and hyphens.",
				InitialValue:      worker.Spec.Namespace,
				Readonly:          true,
			},
			&forms.Field{
				Name:            "kubeconfig",
				Label:           "Kubeconfig",
				Placeholder:     "Enter kubeconfig YAML",
				WidgetComponent: forms.TextWidget,
				Rows:            10,
				AutoResize:      true,
			},
			&forms.Field{
				Name:         "nodeSelector",
				Label:        "Node Selector",
				Placeholder:  "key1=value1,key2=value2",
				InitialValue: formatNodeSelector(worker.Spec.NodeSelector),
			},
			&forms.Field{
				Name:         "tolerations",
				Label:        "Tolerations",
				Placeholder:  "key1=value1:NoSchedule,key2=value2",
				InitialValue: formatTolerations(worker.Spec.Tolerations),
			},
		},
	}

	return form, func() (*gourl.URL, error) {
		object := r.FormValue("object")
		name := strings.Split(object, "/")[1]

		data := form.GetCleanedData()

		// Validate name hasn't changed
		if data["name"] != name {
			err := forms.FieldErrorf(nil, "Worker name cannot be changed")
			form.AddFieldError("name", err)
			return nil, err
		}

		// Validate namespace hasn't changed
		if data["namespace"] != worker.Spec.Namespace {
			err := forms.FieldErrorf(nil, "Worker namespace cannot be changed")
			form.AddFieldError("namespace", err)
			return nil, err
		}

		key := client.ObjectKey{
			Namespace: c.ControllerNamespace,
			Name:      name,
		}

		existing := &apev1.Worker{}
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

		// Update node selector
		existing.Spec.NodeSelector = parseNodeSelector(data["nodeSelector"])

		// Update tolerations
		existing.Spec.Tolerations = parseTolerations(data["tolerations"])

		// Handle kubeconfig secret
		kubeconfigData := data["kubeconfig"]
		if kubeconfigData != "" {
			if existing.Spec.KubeconfigSecret == nil {
				// Create a new secret
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: existing.Name + "-kubeconfig-",
						Namespace:    existing.Namespace,
					},
					Data: map[string][]byte{
						"kubeconfig": []byte(kubeconfigData),
					},
					Type: corev1.SecretTypeOpaque,
				}
				err := c.Create(r.Context(), secret)
				if err != nil {
					form.AddNonFieldError(err)
					return nil, err
				}

				existing.Spec.KubeconfigSecret = &corev1.SecretReference{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				}
			} else {
				// Update existing secret
				secret := &corev1.Secret{}
				err := c.Get(r.Context(), client.ObjectKey{
					Namespace: existing.Namespace,
					Name:      existing.Spec.KubeconfigSecret.Name,
				}, secret)
				if err != nil {
					form.AddNonFieldError(err)
					return nil, err
				}

				secret.Data = map[string][]byte{
					"kubeconfig": []byte(kubeconfigData),
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

func (c *WorkersController) CreateForm(r *http.Request, w datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Section{
				Section: name.Section,
				Fields: forms.FieldList{
					&forms.Field{
						Name:        "displayName",
						Label:       "Location Name",
						Placeholder: "Location display name",
					},
					&forms.Field{
						Name:              "name",
						Label:             "Location Resource Name",
						Description:       "The name of the Location resource in Kubernetes. This cannot be changed.",
						Required:          true,
						Placeholder:       "Enter location resource name (eg. do-ams3-1)",
						MaxLength:         63,
						Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
						PatternInvalidMsg: "Worker name must start with a letter and contain only lowercase letters, numbers, and hyphens.",
					},
				},
			},
			&forms.Field{
				Name:              "namespace",
				Label:             "Worker Namespace",
				Required:          true,
				Placeholder:       "Enter worker namespace (eg. orderly-ape-k6)",
				Description:       "The namespace in the target cluster where the worker will operate.",
				MaxLength:         63,
				Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
				PatternInvalidMsg: "Namespace must start with a letter and contain only lowercase letters, numbers, and hyphens.",
			},
			&forms.Field{
				Name:            "kubeconfig",
				Label:           "Kubeconfig",
				Placeholder:     "Enter kubeconfig YAML",
				WidgetComponent: forms.TextWidget,
				Rows:            10,
				AutoResize:      true,
			},
			&forms.Field{
				Name:        "nodeSelector",
				Label:       "Node Selector",
				Placeholder: "key1=value1,key2=value2",
			},
			&forms.Field{
				Name:        "tolerations",
				Label:       "Tolerations",
				Placeholder: "key1=value1:NoSchedule,key2=value2",
			},
		},
	}

	return form, func() (*gourl.URL, error) {
		data := form.GetCleanedData()

		worker := &apev1.Worker{
			ObjectMeta: metav1.ObjectMeta{
				Name:        data["name"],
				Namespace:   c.ControllerNamespace,
				Annotations: map[string]string{},
			},
			Spec: apev1.WorkerSpec{
				Namespace: data["namespace"],
			},
		}

		if displayName, ok := data["displayName"]; ok && displayName != "" {
			worker.Annotations[common.DisplayNameAnnotation] = displayName
		}

		// Parse node selector
		worker.Spec.NodeSelector = parseNodeSelector(data["nodeSelector"])

		// Parse tolerations
		worker.Spec.Tolerations = parseTolerations(data["tolerations"])

		// Create secret for kubeconfig
		kubeconfigData := data["kubeconfig"]
		if kubeconfigData != "" {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: worker.Name + "-kubeconfig-",
					Namespace:    worker.Namespace,
				},
				Data: map[string][]byte{
					"kubeconfig": []byte(kubeconfigData),
				},
				Type: corev1.SecretTypeOpaque,
			}
			err := c.Create(r.Context(), secret)
			if err != nil {
				form.AddNonFieldError(err)
				return nil, err
			}

			worker.Spec.KubeconfigSecret = &corev1.SecretReference{
				Name:      secret.Name,
				Namespace: worker.Namespace,
			}
		}

		err := c.Create(r.Context(), worker)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				form.AddFieldError("name", forms.FieldErrorf(err, "Worker with this name already exists"))
			} else {
				form.AddNonFieldError(err)
			}
			return nil, err
		}

		return nil, nil
	}, nil
}

func (c *WorkersController) DeleteObjectByName(r *http.Request, name client.ObjectKey) error {
	worker := &apev1.Worker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
	}
	err := c.Get(r.Context(), client.ObjectKeyFromObject(worker), worker)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	if !worker.DeletionTimestamp.IsZero() {
		return nil
	}

	// Delete associated secret if it exists
	if worker.Spec.KubeconfigSecret != nil {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      worker.Spec.KubeconfigSecret.Name,
				Namespace: worker.Namespace,
			},
		}
		_ = c.Delete(context.Background(), secret)
	}

	return c.Delete(r.Context(), worker)
}
