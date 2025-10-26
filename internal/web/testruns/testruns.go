// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testruns

import (
	"context"
	"net/http"
	gourl "net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/util/common"
	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
	"github.com/ReviewSignal/orderly-ape/internal/web/entity"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
	"github.com/ReviewSignal/orderly-ape/internal/web/testruns/testrunutil"
	"github.com/ReviewSignal/orderly-ape/web/components/templui/table"
	"github.com/ReviewSignal/orderly-ape/web/utils"
)

var _ datatable.DataTabler[entity.TestRun] = &TestRunsController{}
var _ datatable.DataLister[entity.TestRun] = &TestRunsController{}
var _ datatable.DataCreator[entity.TestRun] = &TestRunsController{}
var _ datatable.DataEditor[entity.TestRun] = &TestRunsController{}
var _ datatable.DataDeleter[entity.TestRun] = &TestRunsController{}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testscenarios,verbs=get;list;watch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=influxdbs,verbs=get;list;watch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers,verbs=get;list;watch

type TestRunsController struct {
	client.Client
	ControllerNamespace string
}

func (c *TestRunsController) ViewSet() *datatable.ViewSet[entity.TestRun] {
	vs := datatable.NewViewSet[entity.TestRun](c, "testruns", datatable.ViewSetOptions{
		InlineCreate: true,
		InlineEdit:   true,
	})

	actions := []datatable.ObjectAction{
		{
			Name:  "start",
			Label: "Start",
			Icon:  IconStart,
			Available: func(r *http.Request, e datatable.Entry) bool {
				tr, ok := e.(entity.TestRun)
				if e == nil || !ok || tr.TestRun == nil {
					return false
				}
				return tr.Spec.State == apev1.TestRunStateDraft
			},
			InlineForm: func(r *http.Request, e datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
				return nil, func() (*gourl.URL, error) {
					testRun := &apev1.TestRun{}
					err := c.Get(r.Context(), client.ObjectKeyFromObject(e), testRun)
					if err != nil {
						return nil, err
					}
					if testRun.Spec.State != apev1.TestRunStateDraft {
						return nil, forms.FieldErrorf(nil, "Only draft test runs can be started")
					}

					// Get the scenario to merge locations, envVars, and labels
					scenario := &apev1.TestScenario{}
					if testRun.Spec.Scenario != nil {
						err := c.Get(r.Context(), client.ObjectKey{
							Namespace: testRun.Spec.Scenario.Namespace,
							Name:      testRun.Spec.Scenario.Name,
						}, scenario)
						if err != nil {
							return nil, err
						}

						// If TestRun Locations list is empty, copy from TestScenario
						if len(testRun.Spec.Workers) == 0 && len(scenario.Spec.Workers) > 0 {
							testRun.Spec.Workers = scenario.Spec.Workers
						}

						// Merge EnvVars with TestScenario, TestRun having priority
						testRun.Spec.EnvVars = testrunutil.MergeEnvVars(scenario.Spec.EnvVars, testRun.Spec.EnvVars)

						// Merge Labels with TestScenario, TestRun having priority
						testRun.Spec.Labels = testrunutil.MergeLabels(scenario.Spec.Labels, testRun.Spec.Labels)
					}

					testRun.Spec.State = apev1.TestRunStateActive
					err = c.Update(r.Context(), testRun)
					if err != nil {
						return nil, err
					}
					return nil, nil
				}, nil
			},
		},
		{
			Name:  "cancel",
			Label: "Cancel",
			Icon:  IconStop,
			Available: func(r *http.Request, e datatable.Entry) bool {
				tr, ok := e.(entity.TestRun)
				if e == nil || !ok || tr.TestRun == nil {
					return false
				}
				return tr.Spec.State == apev1.TestRunStateActive
			},
			InlineForm: func(r *http.Request, e datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
				return nil, func() (*gourl.URL, error) {
					testRun := &apev1.TestRun{}
					err := c.Get(r.Context(), client.ObjectKeyFromObject(e), testRun)
					if err != nil {
						return nil, err
					}
					if testRun.Spec.State != apev1.TestRunStateActive {
						return nil, forms.FieldErrorf(nil, "Only active test runs can be cancelled")
					}
					testRun.Spec.State = apev1.TestRunStateCanceled
					err = c.Update(r.Context(), testRun)
					if err != nil {
						return nil, err
					}
					return nil, nil
				}, nil
			},
		},
	}
	vs.ObjectActions = append(actions, vs.ObjectActions...)

	return vs
}

func (c *TestRunsController) GetScenarioName(key apev1.ObjectReference) string {
	scenario := &apev1.TestScenario{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: key.Namespace, Name: key.Name}, scenario); err != nil {
		return ""
	}
	return common.DisplayName(scenario)
}

func (c *TestRunsController) GetInfluxDBName(key apev1.ObjectReference) string {
	influx := &apev1.InfluxDB{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: key.Namespace, Name: key.Name}, influx); err != nil {
		return ""
	}
	return common.DisplayName(influx)
}

func (c *TestRunsController) GetLocationName(key apev1.ObjectReference) string {
	worker := &apev1.Worker{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: key.Namespace, Name: key.Name}, worker); err != nil {
		return ""
	}
	return common.DisplayName(worker)
}

func (c *TestRunsController) GetColumns() datatable.Columns {
	return []datatable.Column{
		{
			Name: "Name",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return c.TestRunName(o), table.CellProps{}
			},
		},
		{
			Name: "Target",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return TestRunTarget(o), table.CellProps{}
			},
		},
		{
			Name: "Runtime",
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return c.TestRunRuntime(o), table.CellProps{}
			},
		},
		{
			Name: "",
			HeadProps: table.HeadProps{
				Attributes: templ.Attributes{
					"width": "1%",
				},
			},
			Component: func(o datatable.Entry) (templ.Component, table.CellProps) {
				return TestRunDashboard(o), table.CellProps{
					Attributes: templ.Attributes{
						"width": "1%",
					},
				}
			},
		},
	}
}

func filterTestRuns(testruns []apev1.TestRun, r *http.Request) entity.TestRunList {
	filtered := make(entity.TestRunList, 0, len(testruns))

	search := strings.ToLower(datatable.SearchTerm(r))

	for i, tr := range testruns {
		displayName := common.DisplayName(&tr)
		if strings.Contains(strings.ToLower(tr.Name), search) ||
			strings.Contains(strings.ToLower(displayName), search) ||
			strings.Contains(strings.ToLower(tr.Spec.Target), search) {
			filtered = append(filtered, entity.WrapTestRun(&testruns[i]))
		}
	}

	return filtered
}

func (c *TestRunsController) ListObjects(r *http.Request) ([]entity.TestRun, *datatable.DataPaginator, error) {
	list := &apev1.TestRunList{}
	err := c.List(r.Context(), list, client.InNamespace(c.ControllerNamespace), client.UnsafeDisableDeepCopyOption(true))
	if err != nil {
		return nil, nil, err
	}

	filtered := filterTestRuns(list.Items, r)
	slices.SortStableFunc(filtered, func(a, b entity.TestRun) int {
		return b.CreationTimestamp.Compare(a.CreationTimestamp.Time)
	})

	testrunsIter, totalPages, currentPage, _ := utils.PaginateRequest(r, filtered, datatable.ItemsPerPage)
	testruns := slices.Collect(testrunsIter)

	return testruns, &datatable.DataPaginator{TotalPages: totalPages, CurrentPage: currentPage}, nil
}

func (c *TestRunsController) scenarioChoicer(ctx context.Context) (forms.Choicer, error) {
	list := &apev1.TestScenarioList{}
	if err := c.List(ctx, list); err != nil {
		return nil, err
	}
	return forms.ChoiceFunc(func() []forms.Choice {
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

func (c *TestRunsController) workerChoicer(ctx context.Context) (forms.Choicer, error) {
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

func (c *TestRunsController) influxdbChoicerFromList(list *apev1.InfluxDBList) (forms.Choicer, error) { // nolint:unparam
	return forms.ChoiceFunc(func() []forms.Choice {
		choices := make([]forms.Choice, 0, len(list.Items))
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

func (c *TestRunsController) bucketChoicerField(influx string, list []string) *forms.Field {
	var choicer forms.Choicer = forms.ChoiceFunc(func() []forms.Choice {
		choices := make([]forms.Choice, 0, len(list))
		for _, item := range list {
			if strings.HasPrefix(item, "_") {
				continue
			}
			choices = append(choices, forms.Choice{
				Value:  item,
				Label:  item,
				Object: item,
			})
		}
		return choices
	})
	defaultValue := forms.ChoicerDefault(choicer, forms.ChoicerDefaultFallbackFirst, "default")

	return &forms.Field{
		Name:         "influxdb_bucket[" + influx + "]",
		Label:        "InfluxDB Bucket",
		Required:     false,
		Placeholder:  "Select a bucket",
		Choicer:      choicer,
		InitialValue: defaultValue,
	}
}

func (c *TestRunsController) CreateForm(r *http.Request, _ datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	scenarioChoicer, err := c.scenarioChoicer(r.Context())
	if err != nil {
		return nil, nil, err
	}
	defaultScenarioChoice := forms.ChoicerDefault(scenarioChoicer, forms.ChoicerDefaultFallbackFirst)

	// Get list of Workers for location selection
	workerList := &apev1.WorkerList{}
	if err := c.List(r.Context(), workerList); err != nil {
		return nil, nil, err
	}

	workerChoicer, err := c.workerChoicer(r.Context())
	if err != nil {
		return nil, nil, err
	}

	// Get list of InfluxDBs for bucket choosers
	influxDBList := &apev1.InfluxDBList{}
	if err := c.List(r.Context(), influxDBList); err != nil {
		return nil, nil, err
	}

	influxdbChoicer, err := c.influxdbChoicerFromList(influxDBList)
	if err != nil {
		return nil, nil, err
	}
	defaultInfluxDBChoice := forms.ChoicerDefault(influxdbChoicer, forms.ChoicerDefaultFallbackFirst, "default")

	influxFields := make([]*forms.Field, 0, len(influxDBList.Items)+1)
	influxFields = append(influxFields, &forms.Field{
		Name:         "influxdb",
		Label:        "InfluxDB",
		Required:     true,
		Placeholder:  "Select an InfluxDB instance",
		Choicer:      influxdbChoicer,
		InitialValue: defaultInfluxDBChoice,
	})
	for _, influx := range influxDBList.Items {
		influxFields = append(influxFields, c.bucketChoicerField(influx.Namespace+"/"+influx.Name, influx.Status.Buckets))
	}

	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Field{
				Name:        "target",
				Label:       "Target",
				Required:    true,
				Placeholder: "https://example.com",
			},
			&forms.Field{
				Name:         "scenario",
				Label:        "Scenario",
				Required:     true,
				Placeholder:  "Select a test scenario",
				Choicer:      scenarioChoicer,
				InitialValue: defaultScenarioChoice,
			},
			&forms.Section{
				Title:   "",
				Section: InfluxDBSelector,
				Fields:  influxFields,
			},
			&forms.Field{
				Name:            "locations",
				Label:           "Locations",
				Description:     "If left empty, locations from the selected scenario will be used.",
				WidgetComponent: testrunutil.TestLocationsWidget(workerChoicer),
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

		// Parse scenario reference
		scenarioParts := strings.Split(data["scenario"], "/")
		if len(scenarioParts) != 2 {
			err := forms.FieldErrorf(nil, "Invalid scenario format")
			form.AddFieldError("scenario", err)
			return nil, err
		}

		// Parse influxdb reference
		influxdbParts := strings.Split(data["influxdb"], "/")
		if len(influxdbParts) != 2 {
			err := forms.FieldErrorf(nil, "Invalid influxdb format")
			form.AddFieldError("influxdb", err)
			return nil, err
		}

		// Find selected InfluxDB object
		var selectedInfluxDB *apev1.InfluxDB
		for i := range influxDBList.Items {
			influx := influxDBList.Items[i]
			if influx.Namespace+"/"+influx.Name == data["influxdb"] {
				selectedInfluxDB = &influx
			}
		}
		if selectedInfluxDB == nil {
			err := forms.FieldErrorf(nil, "Selected InfluxDB not found")
			form.AddFieldError("influxdb", err)
			return nil, err
		}

		// Get and validate bucket from the appropriate field
		bucketFieldName := "influxdb_bucket[" + data["influxdb"] + "]"
		bucket := data[bucketFieldName]
		if !slices.Contains(selectedInfluxDB.Status.Buckets, bucket) {
			err := forms.FieldErrorf(nil, "Selected bucket '%s' not found in InfluxDB", bucket)
			form.AddFieldError("influxdb", err)
			return nil, err
		}

		testRun := &apev1.TestRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: FromURL(data["target"]),
				Namespace:    c.ControllerNamespace,
				Labels: map[string]string{
					common.KubernetesAppManagedBy: common.OrderlyApeManager,
				},
			},
			Spec: apev1.TestRunSpec{
				Target: data["target"],
				Scenario: &apev1.ObjectReference{
					Name:      scenarioParts[1],
					Namespace: scenarioParts[0],
				},
				InfluxDB: &apev1.ObjectReference{
					Name:      influxdbParts[1],
					Namespace: influxdbParts[0],
				},
				State: "Draft",
			},
		}

		// Parse and set Workers (Locations)
		locations := make([]testrunutil.Location, 0)
		if data["locations"] != "" {
			if err := json.Unmarshal([]byte(data["locations"]), &locations); err != nil {
				err := forms.FieldErrorf(err, "Invalid locations data")
				form.AddFieldError("locations", err)
				return nil, err
			}
		}
		locationSet := make(map[string]struct{})
		workers := make([]apev1.TestRunWorkerSpec, 0, len(locations))
		for _, l := range locations {
			if l.Name == "" {
				err := forms.FieldErrorf(nil, "Location name is required")
				form.AddFieldError("locations", err)
				return nil, err
			}

			if l.NumWorkers < 1 {
				err := forms.FieldErrorf(nil, "Number of workers must be at least 1")
				form.AddFieldError("locations", err)
				return nil, err
			}

			workerParts := strings.Split(l.Name, "/")
			if len(workerParts) != 2 {
				err := forms.FieldErrorf(nil, "Invalid worker format")
				form.AddFieldError("locations", err)
				return nil, err
			}

			if _, exists := locationSet[l.Name]; exists {
				err := forms.FieldErrorf(nil, "Duplicate location: %s", workerParts[1])
				form.AddFieldError("locations", err)
				return nil, err
			}

			locationSet[l.Name] = struct{}{}
			workers = append(workers, apev1.TestRunWorkerSpec{
				ObjectReference: apev1.ObjectReference{
					Name:      workerParts[1],
					Namespace: workerParts[0],
				},
				NumWorkers: l.NumWorkers,
			})
		}
		testRun.Spec.Workers = workers

		// Parse and set EnvVars
		envVars, err := testrunutil.ParseEnvVars(data, form)
		if err != nil {
			return nil, err
		}
		testRun.Spec.EnvVars = envVars

		// Parse and set Labels
		labels, err := testrunutil.ParseLabels(data, form)
		if err != nil {
			return nil, err
		}
		testRun.Spec.Labels = labels

		// Set influxdb bucket if provided
		if bucket != "" {
			testRun.Spec.InfluxDBBucket = &bucket
		}

		err = c.Create(r.Context(), testRun)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				form.AddNonFieldError(forms.Error("TestRun with this name already exists"))
			} else {
				form.AddNonFieldError(err)
			}
			return nil, err
		}

		return nil, nil
	}, nil
}

func (c *TestRunsController) StartTest(r *http.Request, name client.ObjectKey) error {
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

func (c *TestRunsController) DeleteObjectByName(r *http.Request, name client.ObjectKey) error {
	scenario := &apev1.TestRun{
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

// nolint:gocyclo
func (c *TestRunsController) EditForm(r *http.Request, tr datatable.Entry) (*forms.Form, datatable.FormHandlerFunc, error) {
	testRun, ok := tr.(entity.TestRun)
	if !ok {
		return nil, nil, apierrors.NewBadRequest("Invalid test run entry")
	}

	// Determine if the test run is editable (only Draft state is editable)
	isEditable := testRun.Spec.State == apev1.TestRunStateDraft

	// Get scenario choicer
	scenarioChoicer, err := c.scenarioChoicer(r.Context())
	if err != nil {
		return nil, nil, err
	}

	// Get initial scenario value
	var scenarioValue string
	if testRun.Spec.Scenario != nil {
		scenarioValue = testRun.Spec.Scenario.Namespace + "/" + testRun.Spec.Scenario.Name
	}

	// Fetch the TestScenario to display its Locations, EnvVars, and Labels
	scenario := &apev1.TestScenario{}
	if testRun.Spec.Scenario != nil {
		_ = c.Get(r.Context(), client.ObjectKey{
			Namespace: testRun.Spec.Scenario.Namespace,
			Name:      testRun.Spec.Scenario.Name,
		}, scenario)
	}

	// Get list of Workers for location selection
	workerList := &apev1.WorkerList{}
	if err := c.List(r.Context(), workerList); err != nil {
		return nil, nil, err
	}

	workerChoicer, err := c.workerChoicer(r.Context())
	if err != nil {
		return nil, nil, err
	}

	// Serialize initial values for locations
	var locationsValue string
	if len(testRun.Spec.Workers) > 0 {
		locations := make([]testrunutil.Location, 0, len(testRun.Spec.Workers))
		for _, worker := range testRun.Spec.Workers {
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
	if len(testRun.Spec.EnvVars) > 0 {
		type envVar struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		envVars := make([]envVar, 0, len(testRun.Spec.EnvVars))
		for _, ev := range testRun.Spec.EnvVars {
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
	if len(testRun.Spec.Labels) > 0 {
		type label struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		labels := make([]label, 0, len(testRun.Spec.Labels))
		for k, v := range testRun.Spec.Labels {
			labels = append(labels, label{
				Name:  k,
				Value: v,
			})
		}
		if labelsJSON, err := json.Marshal(labels); err == nil {
			labelsValue = string(labelsJSON)
		}
	}

	// Get list of InfluxDBs for bucket choosers
	influxDBList := &apev1.InfluxDBList{}
	if err := c.List(r.Context(), influxDBList); err != nil {
		return nil, nil, err
	}

	influxdbChoicer, err := c.influxdbChoicerFromList(influxDBList)
	if err != nil {
		return nil, nil, err
	}

	// Get initial influxdb value
	var influxdbValue string
	if testRun.Spec.InfluxDB != nil {
		influxdbValue = testRun.Spec.InfluxDB.Namespace + "/" + testRun.Spec.InfluxDB.Name
	}

	influxFields := make([]*forms.Field, 0, len(influxDBList.Items)+1)
	influxFields = append(influxFields, &forms.Field{
		Name:         "influxdb",
		Label:        "InfluxDB",
		Required:     true,
		Placeholder:  "Select an InfluxDB instance",
		Choicer:      influxdbChoicer,
		InitialValue: influxdbValue,
		Readonly:     !isEditable,
	})

	for _, influx := range influxDBList.Items {
		bucketField := c.bucketChoicerField(influx.Namespace+"/"+influx.Name, influx.Status.Buckets)
		bucketField.Readonly = !isEditable
		// Set initial value for the current influxdb
		if testRun.Spec.InfluxDB != nil &&
			influx.Namespace == testRun.Spec.InfluxDB.Namespace &&
			influx.Name == testRun.Spec.InfluxDB.Name &&
			testRun.Spec.InfluxDBBucket != nil {
			bucketField.InitialValue = *testRun.Spec.InfluxDBBucket
		}
		influxFields = append(influxFields, bucketField)
	}

	form := &forms.Form{
		Fields: []forms.FormItem{
			&forms.Field{
				Name:         "target",
				Label:        "Target",
				Required:     true,
				Placeholder:  "https://example.com",
				InitialValue: testRun.Spec.Target,
				Readonly:     !isEditable,
			},
			&forms.Field{
				Name:         "scenario",
				Label:        "Scenario",
				Required:     true,
				Placeholder:  "Select a test scenario",
				Choicer:      scenarioChoicer,
				InitialValue: scenarioValue,
				Readonly:     !isEditable,
			},
			&forms.Section{
				Title:   "",
				Section: InfluxDBSelector,
				Fields:  influxFields,
			},
		},
	}

	if isEditable {
		form.Fields = append(form.Fields,
			&forms.Field{
				Name:                 "locations",
				Label:                "Locations",
				Description:          "If left empty, locations from the selected scenario will be used.",
				InitialValue:         locationsValue,
				WidgetComponent:      testrunutil.TestLocationsWidget(workerChoicer),
				DescriptionComponent: ScenarioLocationsWidget(scenario.Spec.Workers, c.GetLocationName),
			},
			&forms.Field{
				Name:                 "env",
				Label:                "Env vars",
				Description:          "These override or merge with the scenario's env vars.",
				InitialValue:         envVarsValue,
				WidgetComponent:      testrunutil.TestEnvVarsWidget,
				DescriptionComponent: ScenarioEnvVarsWidget(scenario.Spec.EnvVars),
			},
			&forms.Field{
				Name:                 "labels",
				Label:                "Labels",
				Description:          "These override or merge with the scenario's labels.",
				InitialValue:         labelsValue,
				WidgetComponent:      testrunutil.TestLabelsWidget,
				DescriptionComponent: ScenarioLabelsWidget(scenario.Spec.Labels),
			},
		)
	} else {
		form.Fields = append(form.Fields,
			&forms.Field{
				Name:            "locations",
				Label:           "Locations",
				WidgetComponent: TestRunLocationsDisplayWidget(testRun.Spec.Workers, c.GetLocationName),
			},
			&forms.Field{
				Name:            "env",
				Label:           "Env vars",
				WidgetComponent: TestRunEnvVarsDisplayWidget(testRun.Spec.EnvVars),
			},
			&forms.Field{
				Name:            "labels",
				Label:           "Labels",
				WidgetComponent: TestRunLabelsDisplayWidget(testRun.Spec.Labels),
			},
		)
	}

	return form, func() (*gourl.URL, error) {
		// If not editable, don't allow updates
		if !isEditable {
			err := forms.FieldErrorf(nil, "Only draft test runs can be edited")
			form.AddNonFieldError(err)
			return nil, err
		}

		data := form.GetCleanedData()

		// Get the existing test run
		key := client.ObjectKey{
			Namespace: testRun.GetNamespace(),
			Name:      testRun.GetName(),
		}

		existing := &apev1.TestRun{}
		err := c.Get(r.Context(), key, existing)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		// Verify it's still in Draft state
		if existing.Spec.State != apev1.TestRunStateDraft {
			err := forms.FieldErrorf(nil, "Only draft test runs can be edited")
			form.AddNonFieldError(err)
			return nil, err
		}

		// Parse scenario reference
		scenarioParts := strings.Split(data["scenario"], "/")
		if len(scenarioParts) != 2 {
			err := forms.FieldErrorf(nil, "Invalid scenario format")
			form.AddFieldError("scenario", err)
			return nil, err
		}

		// Parse influxdb reference
		influxdbParts := strings.Split(data["influxdb"], "/")
		if len(influxdbParts) != 2 {
			err := forms.FieldErrorf(nil, "Invalid influxdb format")
			form.AddFieldError("influxdb", err)
			return nil, err
		}

		// Find selected InfluxDB object
		var selectedInfluxDB *apev1.InfluxDB
		for i := range influxDBList.Items {
			influx := influxDBList.Items[i]
			if influx.Namespace+"/"+influx.Name == data["influxdb"] {
				selectedInfluxDB = &influx
			}
		}
		if selectedInfluxDB == nil {
			err := forms.FieldErrorf(nil, "Selected InfluxDB not found")
			form.AddFieldError("influxdb", err)
			return nil, err
		}

		// Get and validate bucket from the appropriate field
		bucketFieldName := "influxdb_bucket[" + data["influxdb"] + "]"
		bucket := data[bucketFieldName]
		if !slices.Contains(selectedInfluxDB.Status.Buckets, bucket) {
			err := forms.FieldErrorf(nil, "Selected bucket '%s' not found in InfluxDB", bucket)
			form.AddFieldError("influxdb", err)
			return nil, err
		}

		// Update the spec
		existing.Spec.Target = data["target"]
		existing.Spec.Scenario = &apev1.ObjectReference{
			Name:      scenarioParts[1],
			Namespace: scenarioParts[0],
		}
		existing.Spec.InfluxDB = &apev1.ObjectReference{
			Name:      influxdbParts[1],
			Namespace: influxdbParts[0],
		}

		// Parse and set Workers (Locations)
		locations := make([]testrunutil.Location, 0)
		if data["locations"] != "" {
			if err := json.Unmarshal([]byte(data["locations"]), &locations); err != nil {
				err := forms.FieldErrorf(err, "Invalid locations data")
				form.AddFieldError("locations", err)
				return nil, err
			}
		}
		locationSet := make(map[string]struct{})
		workers := make([]apev1.TestRunWorkerSpec, 0, len(locations))
		for _, l := range locations {
			if l.Name == "" {
				err := forms.FieldErrorf(nil, "Location name is required")
				form.AddFieldError("locations", err)
				return nil, err
			}

			if l.NumWorkers < 1 {
				err := forms.FieldErrorf(nil, "Number of workers must be at least 1")
				form.AddFieldError("locations", err)
				return nil, err
			}

			workerParts := strings.Split(l.Name, "/")
			if len(workerParts) != 2 {
				err := forms.FieldErrorf(nil, "Invalid worker format")
				form.AddFieldError("locations", err)
				return nil, err
			}

			if _, exists := locationSet[l.Name]; exists {
				err := forms.FieldErrorf(nil, "Duplicate location: %s", workerParts[1])
				form.AddFieldError("locations", err)
				return nil, err
			}

			locationSet[l.Name] = struct{}{}
			workers = append(workers, apev1.TestRunWorkerSpec{
				ObjectReference: apev1.ObjectReference{
					Name:      workerParts[1],
					Namespace: workerParts[0],
				},
				NumWorkers: l.NumWorkers,
			})
		}
		existing.Spec.Workers = workers

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

		// Set influxdb bucket if provided
		if bucket != "" {
			existing.Spec.InfluxDBBucket = &bucket
		}

		err = c.Update(r.Context(), existing)
		if err != nil {
			form.AddNonFieldError(err)
			return nil, err
		}

		return nil, nil
	}, nil
}
