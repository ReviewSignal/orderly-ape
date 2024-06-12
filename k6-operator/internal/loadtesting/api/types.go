package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"

	"github.com/ReviewSignal/loadtesting/k6-operator/api/v1alpha1"
)

const (
	STATUS_PENDING   string = "pending"
	STATUS_QUEUED    string = "queued"
	STATUS_READY     string = "ready"
	STATUS_RUNNING   string = "running"
	STATUS_COMPLETED string = "completed"
	STATUS_FAILED    string = "failed"
)

type NodeSelector map[string]string

type TestRun struct {
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	Target         string            `json:"target"`
	SourceRepo     string            `json:"source_repo"`
	SourceRef      string            `json:"source_ref"`
	SourceScript   string            `json:"source_script"`
	Segments       []string          `json:"segments"`
	Completed      bool              `json:"completed"`
	Ready          bool              `json:"ready"`
	StartTestAt    *time.Time        `json:"start_test_at"`
	ResourceCPU    resource.Quantity `json:"resources_cpu"`
	ResourceMemory resource.Quantity `json:"resources_memory"`
	NodeSelector   NodeSelector      `json:"node_selector"`
	DedicatedNodes bool              `json:"dedicated_nodes"`
}

// Job is a struct that represents a job to be executed by the worker.
// It is exposed by the web application trough the workers API.
type Job struct {
	Name              string   `json:"name"`
	URL               string   `json:"url"`
	Location          string   `json:"location"`
	Status            string   `json:"status"`
	StatusDescription string   `json:"status_description"`
	Workers           int32    `json:"num_workers"`
	OnlineWorkers     int32    `json:"online_workers"`
	AssignedSegments  []string `json:"assigned_segments"`
	TestRun           TestRun  `json:"test_run"`
}

type JobList struct {
	Items []Job `json:"items"`
}

func (o *JobList) ToK8SResource() client.Object {
	panic("Should not be used, so not implemented!")
}

func (o *JobList) GetItem() runtime.Object {
	return &Job{}
}

func (o *JobList) SetItems(items []runtime.Object) {
	o.Items = make([]Job, len(items))
	for i, item := range items {
		o.Items[i] = *(item.(*Job))
	}
}

func (o *Job) GetName() string {
	return o.Name
}

func (o *Job) GetNamespace() string {
	return "default"
}

func (o *Job) ToK8SResource() client.Object {
	t := v1alpha1.TestRun{}

	t.ObjectMeta.Name = o.GetName()
	t.ObjectMeta.Namespace = o.GetNamespace()

	t.ObjectMeta.Annotations = map[string]string{
		"loadtesting.reviewsignal.com/url":        o.URL,
		"loadtesting.reviewsignal.com/created_at": o.TestRun.CreatedAt,
		"loadtesting.reviewsignal.com/updated_at": o.TestRun.UpdatedAt,
		"loadtesting.reviewsignal.com/location":   o.Location,
	}
	t.Spec.Target = o.TestRun.Target
	t.Spec.SourceRepo = o.TestRun.SourceRepo
	t.Spec.SourceRef = o.TestRun.SourceRef
	t.Spec.SourceScript = o.TestRun.SourceScript
	t.Spec.Workers = o.Workers
	t.Spec.Segments = o.TestRun.Segments
	t.Spec.AssignedSegments = o.AssignedSegments

	t.Status.Status = o.Status
	t.Status.Description = o.StatusDescription
	t.Status.OnlineWorkers = o.OnlineWorkers

	return &t
}

func (o *Job) FromK8SResource(t *v1alpha1.TestRun) {
	o.Name = t.ObjectMeta.Name
	o.URL = t.ObjectMeta.Annotations["loadtesting.reviewsignal.com/url"]
	o.TestRun = TestRun{
		CreatedAt:    t.ObjectMeta.Annotations["loadtesting.reviewsignal.com/created_at"],
		UpdatedAt:    t.ObjectMeta.Annotations["loadtesting.reviewsignal.com/updated_at"],
		Target:       t.Spec.Target,
		SourceRepo:   t.Spec.SourceRepo,
		SourceRef:    t.Spec.SourceRef,
		SourceScript: t.Spec.SourceScript,
		Segments:     t.Spec.Segments,
	}
	o.Location = t.ObjectMeta.Annotations["loadtesting.reviewsignal.com/location"]
	o.Workers = t.Spec.Workers
	o.Status = t.Status.Status
	o.StatusDescription = t.Status.Description
	o.OnlineWorkers = t.Status.OnlineWorkers
}

// UnmarshalJSON is a custom unmarshaler for the NodeSelector type.
// It converts from label=value space separated string to a map of label -> value
func (o *NodeSelector) UnmarshalJSON(data []byte) error {
	*o = make(map[string]string)

	selector := ""
	err := json.Unmarshal(data, &selector)
	if err != nil {
		return err
	}

	pairs := strings.Split(selector, " ")
	for _, pair := range pairs {
		if pair == "" {
			continue
		}
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return errors.New("invalid format for key=value pair")
		}
		key := strings.Trim(kv[0], " ")
		value := strings.Trim(kv[1], " ")
		(*o)[key] = value
	}
	return nil

}

func init() {
	runtime.Schema.Register(&Job{}, &JobList{}, "workers/{locationName}/jobs")
}
