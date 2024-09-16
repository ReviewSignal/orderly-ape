package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/options"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

const (
	STATUS_PENDING   string = "pending"
	STATUS_QUEUED    string = "queued"
	STATUS_READY     string = "ready"
	STATUS_RUNNING   string = "running"
	STATUS_CANCELED  string = "canceled"
	STATUS_COMPLETED string = "completed"
	STATUS_FAILED    string = "failed"
)

type Ping struct{}
type PingList struct {
	Items []Ping `json:"items"`
}

func (o *Ping) GetName() string {
	panic("Should not be used, so not implemented!")
}
func (o *Ping) ToK8SResource() client.Object {
	panic("Should not be used, so not implemented!")
}
func (o *PingList) GetItem() runtime.Object {
	panic("Should not be used, so not implemented!")
}
func (o *PingList) SetItems(items []runtime.Object) {
	panic("Should not be used, so not implemented!")
}

type TestRun struct {
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	Target         string            `json:"target"`
	EnvVars        map[string]string `json:"env_vars"`
	Labels         map[string]string `json:"labels"`
	SourceRepo     string            `json:"source_repo"`
	SourceRef      string            `json:"source_ref"`
	SourceScript   string            `json:"source_script"`
	Segments       []string          `json:"segments"`
	Completed      bool              `json:"completed"`
	Ready          bool              `json:"ready"`
	StartTestAt    *time.Time        `json:"started_at"`
	ResourceCPU    resource.Quantity `json:"resources_cpu"`
	ResourceMemory resource.Quantity `json:"resources_memory"`
	NodeSelector   NodeSelector      `json:"node_selector"`
	JobDeadline    *Duration         `json:"job_deadline"`
	DedicatedNodes bool              `json:"dedicated_nodes"`
}

type TestOutputConfig struct {
	InfluxURL          string `json:"influxdb_url"`
	InfluxToken        string `json:"influxdb_token"`
	InfluxOrganization string `json:"influxdb_org"`
	InfluxBucket       string `json:"influxdb_bucket"`
	TLSSkipVerify      bool   `json:"insecure_skip_verify"`
}

type Segment struct {
	ID      string `json:"segment_id"`
	Segment string `json:"segment"`
}

// Job is a struct that represents a job to be executed by the worker.
// It is exposed by the web application trough the workers API.
type Job struct {
	Name              string           `json:"name"`
	URL               string           `json:"url"`
	Location          string           `json:"location"`
	Status            string           `json:"status"`
	StatusDescription string           `json:"status_description"`
	Workers           int32            `json:"num_workers"`
	OnlineWorkers     int32            `json:"online_workers"`
	AssignedSegments  []Segment        `json:"assigned_segments"`
	TestRun           TestRun          `json:"test_run"`
	OutputConfig      TestOutputConfig `json:"output_config"`
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
	return options.JobNamespace
}

func (o *Job) ToK8SResource() client.Object {
	obj := batchv1.Job{}

	obj.ObjectMeta.Name = o.GetName()
	obj.ObjectMeta.Namespace = o.GetNamespace()

	return &obj
}

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}

type NodeSelector map[string]string

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
	runtime.Schema.Register(&Ping{}, &PingList{}, "workers/{locationName}/ping")
	runtime.Schema.Register(&Job{}, &JobList{}, "workers/{locationName}/jobs")
}
