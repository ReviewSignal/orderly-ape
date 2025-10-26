// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testscenarios

import (
	"context"
	"net/http"
	gourl "net/url"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ReviewSignal/orderly-ape/internal/util/common"
	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
	"github.com/ReviewSignal/orderly-ape/internal/web/entity"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/internal/web/testruns/testrunutil"
	"github.com/ReviewSignal/orderly-ape/web/components/bitpoke/name"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/table"
	"github.com/ReviewSignal/orderly-ape/web/utils"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

var _ datatable.DataTabler[entity.TestScenario] = &TestScenariosController{}
var _ datatable.DataLister[entity.TestScenario] = &TestScenariosController{}
var _ datatable.DataCreator[entity.TestScenario] = &TestScenariosController{}
var _ datatable.DataEditor[entity.TestScenario] = &TestScenariosController{}
var _ datatable.DataDeleter[entity.TestScenario] = &TestScenariosController{}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testscenarios,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers,verbs=get;list;watch

type TestScenariosController struct {
	client.Client
	ControllerNamespace   string
	DefaultScenarioCPU    string
	DefaultScenarioMemory string
}

func (c *TestScenariosController) ViewSet() *datatable.ViewSet[entity.TestScenario] {
	return datatable.NewViewSet[entity.TestScenario](c, "testscenarios", datatable.ViewSetOptions{
		InlineCreate: true,
		InlineEdit:   true,
	})
}

func (c *TestScenariosController) GetColumns() datatable.Columns {
	return []datatable.Column{
		{
			Name: "Name",
		},
		{
			Name: "Git Repository",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return TestScenarioGitRepo(o), table.CellProps{}
			},
		},
		{
			Name: "Max Duration",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return TestScenarioMaxDuration(o), table.CellProps{}
			},
		},
		{
			Name: "Locations",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return c.TestScenarioLocations(o), table.CellProps{}
			},
		},
	}
}

func filterTestScenarios(scenarios []apev1.TestScenario, r *http.Request) entity.TestScenarioList {
	filtered := make(entity.TestScenarioList, 0, len(scenarios))

	search := strings.ToLower(datatable.SearchTerm(r))

	for i, ts := range scenarios {
		displayName := common.DisplayName(&ts)
		if strings.Contains(strings.ToLower(ts.Name), search) ||
			strings.Contains(strings.ToLower(displayName), search) {
			filtered = append(filtered, entity.WrapTestScenario(&scenarios[i]))
		}
	}

	return filtered
}

func (c *TestScenariosController) ListObjects(r *http.Request) ([]entity.TestScenario, *datatable.DataPaginator, error) {
	list := &apev1.TestScenarioList{}
	err := c.List(r.Context(), list, client.InNamespace(c.ControllerNamespace))
	if err != nil {
		return nil, nil, err
	}

	filtered := filterTestScenarios(list.Items, r)
	slices.SortStableFunc(filtered, func(a, b entity.TestScenario) int {
		return strings.Compare(a.GetName(), b.GetName())
	})

	scenariosIter, totalPages, currentPage, _ := utils.PaginateRequest(r, filtered, datatable.ItemsPerPage)
	scenarios := slices.Collect(scenariosIter)

	return scenarios, &datatable.DataPaginator{TotalPages: totalPages, CurrentPage: currentPage}, nil
}

func (c *TestScenariosController) workerChoicer(ctx context.Context) (forms.Choicer, error) {
	list := &apev1.WorkerList{}
	if err := c.List(ctx, list); err != nil {
		return nil, err
	}
	return forms.MultiChoiceFunc(func() []forms.Choice {
		choices := make([]forms.Choice, 0)
		for _, item := range list.Items {
			choices = append(choices, forms.Choice{
				Value:  item.Namespace + "/" + item.Name,
				Label:  item.Name,
				Object: item,
			})
		}
		return choices
	}), nil
}

func (c *TestScenariosController) GetLocationName(key apev1.ObjectReference) string {
	worker := &apev1.Worker{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: key.Namespace, Name: key.Name}, worker); err != nil {
		return ""
	}
	return common.DisplayName(worker)
}

//nolint:gocyclo // Function complexity inherited from existing code; refactoring beyond scope
func (c *TestScenariosController) EditForm(r *http.Request, ts datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	scenario, ok := ts.(entity.TestScenario)
	if !ok {
		return nil, nil, apierrors.NewBadRequest("Invalid test scenario entry")
	}

	// Get initial values
	var gitRepo, gitRevision, gitPath, maxDuration string
	if scenario.Spec.GitRepo != nil {
		gitRepo = scenario.Spec.GitRepo.Repository
		gitRevision = scenario.Spec.GitRepo.Revision
		gitPath = scenario.Spec.GitRepo.Path
	}
	if scenario.Spec.MaxDuration != nil {
		maxDuration = scenario.Spec.MaxDuration.Duration.String()
	}

	// Get initial resource values
	var cpuLimit, memoryLimit string
	if scenario.Spec.Resources != nil {
		if cpu, ok := scenario.Spec.Resources.Limits[corev1.ResourceCPU]; ok {
			cpuLimit = cpu.String()
		}
		if memory, ok := scenario.Spec.Resources.Limits[corev1.ResourceMemory]; ok {
			memoryLimit = memory.String()
		}
	}
	// Don't set defaults here - they're shown as placeholders in the form

	workerChoicer, err := c.workerChoicer(r.Context())
	if err != nil {
		return nil, nil, err
	}

	// Serialize initial values for locations
	var locationsValue string
	if len(scenario.Spec.Workers) > 0 {
		locations := make([]testrunutil.Location, 0, len(scenario.Spec.Workers))
		for _, worker := range scenario.Spec.Workers {
			locations = append(locations, testrunutil.Location{
				Name:       worker.Namespace + "/" + worker.Name,
				NumWorkers: worker.NumWorkers,
			})
		}
		if locationsJSON, err := json.Marshal(locations); err == nil {
			locationsValue = string(locationsJSON)
		}
	}

	// Serialize initial values for env vars
	var envVarsValue string
	if len(scenario.Spec.EnvVars) > 0 {
		type envVar struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		envVars := make([]envVar, 0, len(scenario.Spec.EnvVars))
		for _, ev := range scenario.Spec.EnvVars {
			envVars = append(envVars, envVar{
				Name:  ev.Name,
				Value: ev.Value,
			})
		}
		if envVarsJSON, err := json.Marshal(envVars); err == nil {
			envVarsValue = string(envVarsJSON)
		}
	}

	// Serialize initial values for labels
	var labelsValue string
	if len(scenario.Spec.Labels) > 0 {
		type label struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		labels := make([]label, 0, len(scenario.Spec.Labels))
		for k, v := range scenario.Spec.Labels {
			labels = append(labels, label{
				Name:  k,
				Value: v,
			})
		}
		if labelsJSON, err := json.Marshal(labels); err == nil {
			labelsValue = string(labelsJSON)
		}
	}

	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Field{
				Name:         "displayName",
				Label:        "Test Scenario Name",
				Placeholder:  "Test scenario display name",
				InitialValue: common.DisplayName(ts),
			},
			&forms.Field{
				Name:              "name",
				Label:             "Test Scenario Name",
				Description:       "The name of Test Scenario resource in Kubernetes. This cannot be changed.",
				Required:          true,
				Placeholder:       "Enter test scenario name",
				MaxLength:         63,
				Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
				PatternInvalidMsg: "Test Scenatio name must start with a letter and contain only lowercase letters, numbers, and hyphens.",
				InitialValue:      ts.GetName(),
				Readonly:          true,
			},
			&forms.Field{
				Name:         "gitRepository",
				Label:        "Git Repository",
				Required:     true,
				Placeholder:  "https://github.com/ReviewSignal/loadtesting",
				InitialValue: gitRepo,
			},
			&forms.Field{
				Name:         "gitRevision",
				Label:        "Git Revision",
				Placeholder:  "main",
				InitialValue: gitRevision,
			},
			&forms.Field{
				Name:         "gitPath",
				Label:        "Path",
				Placeholder:  "loadtest.js",
				InitialValue: gitPath,
			},
			&forms.Field{
				Name:            "locations",
				Label:           "Locations",
				Required:        true,
				InitialValue:    locationsValue,
				WidgetComponent: testrunutil.TestLocationsWidget(workerChoicer),
			},
			&forms.Field{
				Name:         "maxDuration",
				Label:        "Max Duration",
				Placeholder:  "30m",
				InitialValue: maxDuration,
			},
			&forms.Field{
				Name:         "cpuLimit",
				Label:        "CPU Limit (per k6 worker)",
				Placeholder:  c.DefaultScenarioCPU,
				InitialValue: cpuLimit,
			},
			&forms.Field{
				Name:         "memoryLimit",
				Label:        "Memory Limit (per k6 worker)",
				Placeholder:  c.DefaultScenarioMemory,
				InitialValue: memoryLimit,
			},
			&forms.Field{
				Name:            "env",
				Label:           "Env vars",
				InitialValue:    envVarsValue,
				WidgetComponent: testrunutil.TestEnvVarsWidget,
			},
			&forms.Field{
				Name:            "labels",
				Label:           "Labels",
				InitialValue:    labelsValue,
				WidgetComponent: testrunutil.TestLabelsWidget,
			},
		},
	}

	return form, func() (*gourl.URL, error) {
		object := r.FormValue("object")
		name := strings.Split(object, "/")[1]

		data := form.GetCleanedData()

		// Validate name hasn't changed
		if data["name"] != name {
			err := forms.FieldErrorf(nil, "Test scenario name cannot be changed")
			form.AddFieldError("name", err)
			return nil, err
		}

		key := client.ObjectKey{
			Namespace: c.ControllerNamespace,
			Name:      name,
		}

		existing := &apev1.TestScenario{}
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

		// Update git repository
		gitRepo := data["gitRepository"]
		gitRevision := data["gitRevision"]
		gitPath := data["gitPath"]

		if gitRepo != "" {
			if existing.Spec.GitRepo == nil {
				existing.Spec.GitRepo = &apev1.TestGitRepoSource{}
			}
			existing.Spec.GitRepo.Repository = gitRepo

			if gitRevision != "" {
				existing.Spec.GitRepo.Revision = gitRevision
			} else {
				existing.Spec.GitRepo.Revision = "main"
			}

			if gitPath != "" {
				existing.Spec.GitRepo.Path = gitPath
			} else {
				existing.Spec.GitRepo.Path = "loadtest.js"
			}
		}

		// Update max duration
		if maxDurationStr := data["maxDuration"]; maxDurationStr != "" {
			d, err := time.ParseDuration(maxDurationStr)
			if err != nil {
				form.AddFieldError("maxDuration", forms.FieldErrorf(err, "Invalid duration format"))
				return nil, err
			}
			existing.Spec.MaxDuration = &metav1.Duration{Duration: d}
		}

		// Update resource limits
		if err := updateResourceLimits(&existing.Spec, data, form); err != nil {
			return nil, err
		}

		// Parse and set Workers (Locations)
		workers, err := testrunutil.ParseLocations(data, form)
		if err != nil {
			return nil, err
		}
		if workers != nil {
			existing.Spec.Workers = workers
		}

		// Parse and set EnvVars
		envVars, err := testrunutil.ParseEnvVars(data, form)
		if err != nil {
			return nil, err
		}
		existing.Spec.EnvVars = envVars

		// Parse and set Labels
		labels, err := testrunutil.ParseLabels(data, form)
		if err != nil {
			return nil, err
		}
		existing.Spec.Labels = labels

		err = c.Update(r.Context(), existing)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		return nil, nil
	}, nil
}

func (c *TestScenariosController) CreateForm(r *http.Request, ts datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	workerChoicer, err := c.workerChoicer(r.Context())
	if err != nil {
		return nil, nil, err
	}

	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Section{
				Section: name.Section,
				Fields: forms.FieldList{
					&forms.Field{
						Name:        "displayName",
						Label:       "Test Scenario Name",
						Placeholder: "Test scenario display name",
					},
					&forms.Field{
						Name:              "name",
						Label:             "Test Scenario Resrouce Name",
						Description:       "The name of the Test Scenario resource in Kubernetes. This cannot be changed.",
						Required:          true,
						Placeholder:       "Enter test scenario resource name",
						MaxLength:         63,
						Pattern:           "^[a-z]([a-z0-9\\-]*[a-z0-9])?$",
						PatternInvalidMsg: "Test scenario must start with a letter and contain only lowercase letters, numbers, and hyphens.",
					},
				},
			},
			&forms.Field{
				Name:        "gitRepository",
				Label:       "Git Repository",
				Required:    true,
				Placeholder: "https://github.com/ReviewSignal/loadtesting",
			},
			&forms.Field{
				Name:        "gitRevision",
				Label:       "Git Revision",
				Placeholder: "main",
			},
			&forms.Field{
				Name:        "gitPath",
				Label:       "Path",
				Placeholder: "loadtest.js",
			},
			&forms.Field{
				Name:            "locations",
				Label:           "Locations",
				Required:        true,
				WidgetComponent: testrunutil.TestLocationsWidget(workerChoicer),
			},
			&forms.Field{
				Name:        "maxDuration",
				Label:       "Max Duration",
				Placeholder: "30m",
			},
			&forms.Field{
				Name:        "cpuLimit",
				Label:       "CPU Limit (per k6 worker)",
				Placeholder: c.DefaultScenarioCPU,
			},
			&forms.Field{
				Name:        "memoryLimit",
				Label:       "Memory Limit (per k6 worker)",
				Placeholder: c.DefaultScenarioMemory,
			},
			&forms.Field{
				Name:            "env",
				Label:           "Env vars",
				WidgetComponent: testrunutil.TestEnvVarsWidget,
			},
			&forms.Field{
				Name:            "labels",
				Label:           "Labels",
				WidgetComponent: testrunutil.TestLabelsWidget,
			},
		},
	}

	return form, func() (*gourl.URL, error) {
		data := form.GetCleanedData()

		scenario := &apev1.TestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:        data["name"],
				Namespace:   c.ControllerNamespace,
				Annotations: map[string]string{},
			},
			Spec: apev1.TestScenarioSpec{},
		}

		if displayName, ok := data["displayName"]; ok && displayName != "" {
			scenario.Annotations[common.DisplayNameAnnotation] = displayName
		}

		// Set git repository
		gitRepo := data["gitRepository"]
		if gitRepo != "" {
			scenario.Spec.GitRepo = &apev1.TestGitRepoSource{
				Repository: gitRepo,
				Revision:   "main",
				Path:       "loadtest.js",
			}

			if gitRevision := data["gitRevision"]; gitRevision != "" {
				scenario.Spec.GitRepo.Revision = gitRevision
			}

			if gitPath := data["gitPath"]; gitPath != "" {
				scenario.Spec.GitRepo.Path = gitPath
			}
		}

		// Set max duration
		if maxDurationStr := data["maxDuration"]; maxDurationStr != "" {
			d, err := time.ParseDuration(maxDurationStr)
			if err != nil {
				form.AddFieldError("maxDuration", forms.FieldErrorf(err, "Invalid duration format"))
				return nil, err
			}
			scenario.Spec.MaxDuration = &metav1.Duration{Duration: d}
		}

		// Set resource limits
		if err := updateResourceLimits(&scenario.Spec, data, form); err != nil {
			return nil, err
		}

		// Parse and set Workers (Locations)
		workers, err := testrunutil.ParseLocations(data, form)
		if err != nil {
			return nil, err
		}
		scenario.Spec.Workers = workers

		// Parse and set EnvVars
		envVars, err := testrunutil.ParseEnvVars(data, form)
		if err != nil {
			return nil, err
		}
		scenario.Spec.EnvVars = envVars

		// Parse and set Labels
		labels, err := testrunutil.ParseLabels(data, form)
		if err != nil {
			return nil, err
		}
		scenario.Spec.Labels = labels

		err = c.Create(r.Context(), scenario)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				form.AddFieldError("name", forms.FieldErrorf(err, "Test scenario with this name already exists"))
			} else {
				form.AddNonFieldError(err)
			}
			return nil, err
		}

		return nil, nil
	}, nil
}

func (c *TestScenariosController) DeleteObjectByName(r *http.Request, name client.ObjectKey) error {
	scenario := &apev1.TestScenario{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
	}
	err := c.Get(r.Context(), client.ObjectKeyFromObject(scenario), scenario)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	if !scenario.DeletionTimestamp.IsZero() {
		return nil
	}

	return c.Delete(context.Background(), scenario)
}

// updateResourceLimits updates the resource limits in the spec based on form data.
// Empty values are treated as requests to clear that specific limit.
// To set explicit limits, users must provide values.
func updateResourceLimits(spec *apev1.TestScenarioSpec, data map[string]string, form *forms.Form) error {
	cpuLimitStr := data["cpuLimit"]
	memoryLimitStr := data["memoryLimit"]

	// Initialize Resources struct if needed
	if spec.Resources == nil {
		spec.Resources = &corev1.ResourceRequirements{}
	}
	if spec.Resources.Limits == nil {
		spec.Resources.Limits = corev1.ResourceList{}
	}

	// Handle CPU limit
	if cpuLimitStr != "" {
		cpuQuantity, err := parseResourceQuantity(cpuLimitStr)
		if err != nil {
			form.AddFieldError("cpuLimit", forms.FieldErrorf(err, "Invalid CPU format"))
			return err
		}
		spec.Resources.Limits[corev1.ResourceCPU] = cpuQuantity
	} else {
		// Clear CPU limit if field is empty
		delete(spec.Resources.Limits, corev1.ResourceCPU)
	}

	// Handle memory limit
	if memoryLimitStr != "" {
		memoryQuantity, err := parseResourceQuantity(memoryLimitStr)
		if err != nil {
			form.AddFieldError("memoryLimit", forms.FieldErrorf(err, "Invalid memory format"))
			return err
		}
		spec.Resources.Limits[corev1.ResourceMemory] = memoryQuantity
	} else {
		// Clear memory limit if field is empty
		delete(spec.Resources.Limits, corev1.ResourceMemory)
	}

	// Clean up empty structures (len() on nil maps returns 0)
	if len(spec.Resources.Limits) == 0 && len(spec.Resources.Requests) == 0 {
		spec.Resources = nil
	}

	return nil
}

// parseResourceQuantity parses a resource quantity string (e.g., "1", "2Gi", "500m")
func parseResourceQuantity(value string) (resource.Quantity, error) {
	return resource.ParseQuantity(value)
}
