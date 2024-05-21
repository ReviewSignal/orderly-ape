package api

import (
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"

	"github.com/ReviewSignal/loadtesting/k6-operator/api/v1alpha1"
)

type TestRun struct {
	Name         string   `json:"name"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	Target       string   `json:"target"`
	SourceRepo   string   `json:"source_repo"`
	SourceRef    string   `json:"source_ref"`
	SourceScript string   `json:"source_script"`
	Segments     []string `json:"segments"`
}

// Job is a struct that represents a job to be executed by the worker.
// It is exposed by the web application trough the workers API.
type Job struct {
	ID                int64    `json:"id"`
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

func (o *Job) ToK8SResource() client.Object {
	t := v1alpha1.TestRun{}

	t.ObjectMeta.Name = o.TestRun.Name
	t.ObjectMeta.Namespace = "default"

	t.ObjectMeta.Annotations = map[string]string{
		"loadtesting.reviewsignal.com/id":         fmt.Sprintf("%d", o.ID),
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
	id, _ := strconv.ParseInt(t.ObjectMeta.Annotations["loadtesting.reviewsignal.com/id"], 10, 32)

	o.ID = id
	o.TestRun = TestRun{
		Name:         t.ObjectMeta.Name,
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

func init() {
	runtime.Schema.Register(&Job{}, &JobList{}, "workers/{locationName}/jobs")
}
