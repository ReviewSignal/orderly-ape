// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package v1

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TestRunWorkerSpec defines the specification of a worker in a TestRun.
type TestRunWorkerSpec struct {
	ObjectReference `json:",inline"`

	// NumWorkers is the number of workers to use for this location.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	NumWorkers int32 `json:"numWorkers,omitempty"`
}

const (
	TestRunPlacementStrategySpreadOut     = "SpreadOut"
	TestRunPlacementStrategyAllowSameNode = "AllowSameNode"
)

type TestRunState string

const (
	TestRunStateDraft    TestRunState = "Draft"
	TestRunStateActive   TestRunState = "Active"
	TestRunStateCanceled TestRunState = "Canceled"
	TestRunStateArchived TestRunState = "Archived"
)

const (
	PlacementSpreadOut     = "SpreadOut"
	PlacementAllowSameNode = "AllowSameNode"
)

type EnvVar struct {
	// Name of the environment variable. Must be a C_IDENTIFIER.
	// It cannot be TARGET, or start with K6_ or APE_ as these are reserved for the test scenario and system.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="!matches(self.upperAscii(), '^(TARGET|K6_.*|APE_.*)$')",message="Name cannot be TARGET or start with K6_ or APE_ as these are reserved."
	Name string `json:"name"`

	// Value of the environment variable.
	Value string `json:"value,omitempty"`
}

// TestRunSpec defines the desired state of TestRun
type TestRunSpec struct {
	// Target is the test run target.
	// It get passed to the test scenario as the TARGET environment variable.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Target string `json:"target"`

	// Scenario is a reference to the TestScenario to run.
	// +kubebuilder:validation:Required
	Scenario *ObjectReference `json:"scenario"`

	// InfluxDB is a reference to an InfluxDB for storing test results.
	// +kubebuilder:validation:Required
	InfluxDB *ObjectReference `json:"influxdb,omitempty"`

	// InfluxDBBucket is the name of the InfluxDB bucket to use for storing test results.
	// If not specified, the default bucket from the InfluxDB instance will be used.
	// +optional
	InfluxDBBucket *string `json:"influxdbBucket,omitempty"`

	// Workers is a list of references to Workers (locations) where the test should run.
	// It overrides the locations defined in the TestScenario.
	// +listType=map
	// +listMapKey=name
	// +optional
	Workers []TestRunWorkerSpec `json:"workers,omitempty"`

	// EnvVars is a list of environment variables to set for the test run.
	// They are merged with the environment variables defined in the TestScenario.
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=24
	// +optional
	EnvVars []EnvVar `json:"envVars,omitempty"`

	// Labels are test results labels to set for the test run.
	// They are merged with the labels defined in the TestScenario.
	// +kubebuilder:validation:MaxProperties=24
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// PlacementStrategy is the strategy for placing worker pods in locations.
	// +kubebuilder:validation:Enum=SpreadOut;AllowSameNode
	// +kubebuilder:default=SpreadOut
	PlacementStrategy string `json:"placementStrategy,omitempty"`

	// State is the desired state of the test run.
	// Once Canceled, it cannot be changed back to Active or Draft.
	// +kubebuilder:validation:Enum=Draft;Active;Canceled;Archived
	// +kubebuilder:default=Draft
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="oldSelf == 'Draft' || self == 'Draft' || self == 'Archived' || oldSelf == self || oldSelf == 'Active' && self == 'Canceled'",message="Only Draft can be started or canceled. Only Active can only be canceled."
	State TestRunState `json:"state,omitempty"`
}

type TestRunWorkerStatus struct {
	// Name is the name of the worker.
	Name string `json:"name"`

	// Phase is the current phase of the worker.
	// +kubebuilder:default=Pending
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Phase TestRunPhase `json:"phase"`

	// Conditions is the list of conditions for the worker.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AssignedSegments are the segments assigned to this worker.
	AssignedSegments []string `json:"assignedSegments,omitempty"`
}

// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Enum=Pending;Queued;Ready;Running;Completed;Failed;Canceled;Unknown
//
//go:generate go tool enumer -type=TestRunPhase -trimprefix=TestRunPhase -json
type TestRunPhase int64

// These are the valid phases of a TestRun.
const (
	TestRunPhaseUnknown TestRunPhase = 0
	TestRunPhaseDraft   TestRunPhase = 1 << iota
	TestRunPhasePending
	TestRunPhaseQueued
	TestRunPhaseReady
	TestRunPhaseFailed
	TestRunPhaseCanceling
	TestRunPhaseCanceled
	TestRunPhaseRunning
	TestRunPhaseCompleted
	TestRunPhaseArchived
)

func (s TestRun) Phase() TestRunPhase {
	// Handle special cases first.
	phases := map[TestRunPhase]struct{}{}
	nonFinalPhases := map[TestRunPhase]struct{}{}
	for _, worker := range s.Status.WorkerStatuses {
		phases[worker.Phase] = struct{}{}
		if worker.Phase != TestRunPhaseCompleted && worker.Phase != TestRunPhaseFailed && worker.Phase != TestRunPhaseCanceled {
			nonFinalPhases[worker.Phase] = struct{}{}
		}
	}

	if _, ok := phases[TestRunPhaseFailed]; ok {
		return TestRunPhaseFailed
	}
	if _, ok := phases[TestRunPhaseCanceled]; ok {
		return TestRunPhaseCanceled
	}

	switch {
	case s.Spec.State == "" || s.Spec.State == TestRunStateDraft:
		return TestRunPhaseDraft
	case s.Spec.State == TestRunStateArchived:
		return TestRunPhaseArchived
	case s.Spec.State == TestRunStateCanceled:
		if len(s.Spec.Workers) == 0 || len(nonFinalPhases) == 0 {
			return TestRunPhaseCanceled
		} else {
			return TestRunPhaseCanceling
		}
	case len(s.Status.WorkerStatuses) == 0:
		// No worker statuses yet, return Pending or Draft based on state
		if s.Spec.State == TestRunStateActive {
			return TestRunPhasePending
		}
		return TestRunPhaseDraft
	case len(s.Status.WorkerStatuses) < len(s.Spec.Workers) && s.Spec.State == TestRunStateActive:
		return TestRunPhasePending
	}

	// Return the "lowest" phase of all workers.
	worker := slices.MinFunc(s.Status.WorkerStatuses, func(a, b TestRunWorkerStatus) int {
		if a.Phase < b.Phase {
			return -1
		} else if a.Phase > b.Phase {
			return 1
		} else {
			return 0
		}
	})

	return worker.Phase
}

func (s TestRun) TotalWorkers() int {
	totalWorkers := 0
	for _, worker := range s.Spec.Workers {
		if worker.NumWorkers < 1 {
			totalWorkers += 1
			continue
		}
		totalWorkers += int(worker.NumWorkers)
	}
	return totalWorkers
}

func (s TestRun) SegmentSequence() []string {
	totalWorkers := s.TotalWorkers()
	seq := []string{"0", "1"}
	if s.TotalWorkers() <= 1 {
		return seq
	}
	seq = make([]string, totalWorkers+1)
	for i := 0; i <= totalWorkers; i++ {
		switch i {
		case 0:
			seq[i] = "0"
		case totalWorkers:
			seq[i] = "1"
		default:
			seq[i] = fmt.Sprintf("%d/%d", i, totalWorkers)
		}
	}
	return seq
}

func (s TestRun) Segments() []string {
	seq := s.SegmentSequence()
	num := len(seq) - 1
	if num <= 0 {
		return []string{"0:1"}
	}

	// Build all segment ranges: "segment[i]:segment[i+1]"
	segments := make([]string, num)
	for i := range num {
		segments[i] = seq[i] + ":" + seq[i+1]
	}

	return segments
}

const (
	TestRunWorkerConditionFailed = "Failed"
	TestRunWorkerConditionReady  = "Ready"

	TestRunWorkerReasonCanceled        = "Canceled"
	TestRunWorkerReasonFailed          = "Failed"
	TestRunWorkerReasonCompleted       = "Completed"
	TestRunWorkerReasonConfigError     = "ConfigError"
	TestRunWorkerReasonCancelFailed    = "CancelFailed"
	TestRunWorkerReasonJobNotFound     = "JobNotFound"
	TestRunWorkerReasonJobFailed       = "JobFailed"
	TestRunWorkerReasonJobCreated      = "JobCreated"
	TestRunWorkerReasonJobCreateFailed = "JobCreateFailed"
	TestRunWorkerReasonPodsReady       = "PodsReady"
	TestRunWorkerReasonPodLookupFailed = "PodLookupFailed"
	TestRunWorkerReasonIgniterFailed   = "IgniterFailed"
)

// TestRunScenarioSpec defines scenario values for this test run.
type TestRunScenarioSpec struct {
	TestSource `json:",inline"`

	// Resources defines the resource requests and limits for the test scenario execution.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// MaxDuration is the maximum duration the test scenario is allowed to run.
	// This is a safety measure to prevent runaway tests.
	// It must be a valid duration string. For example: "30m", "1h", "2h45m".
	// If not specified, it defaults to "30m".
	// It must account for the time taken to set up the test scenario (dowloading docker image, worker synchronization
	// time, etc.).
	// +optional
	// +kubebuilder:default="30m"
	MaxDuration *metav1.Duration `json:"maxDuration,omitempty"`
}

// VictoriaLogsConfig defines the VictoriaLogs configuration for a test run.
type VictoriaLogsConfig struct {
	// URL is the VictoriaLogs insert endpoint URL.
	// +optional
	URL string `json:"url,omitempty"`

	// AccountID is the VictoriaLogs account ID.
	// +optional
	AccountID int32 `json:"accountID,omitempty"`

	// ProjectID is the VictoriaLogs project ID assigned to this test run.
	// +optional
	ProjectID int32 `json:"projectID,omitempty"`
}

// TestRunStatus defines the observed state of TestRun.
type TestRunStatus struct {
	// WorkerStatuses is the status of each worker.
	// +listType=map
	// +listMapKey=name
	// +optional
	WorkerStatuses []TestRunWorkerStatus `json:"workerStatuses,omitempty"`

	// ScenarioSpec are the scenario values for this test run.
	// +optional
	ScenarioSpec *TestRunScenarioSpec `json:"scenarioSpec,omitempty"`

	// InfluxDBSecret is a secret generated for each test run with the following keys:
	// INFLUXDB_HOST, INFLUXDB_ORG, INFLUXDB_BUCKET, INFLUXDB_TOKEN
	InfluxDBSecret *ObjectReference `json:"influxdbCredentials,omitempty"`

	// StartTime is the time the test was set to be started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time the test run completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// DashboardUID is the UID of the Grafana dashboard for this test run.
	// +optional
	DashboardUID string `json:"dashboardUID,omitempty"`

	// VictoriaLogs contains the VictoriaLogs configuration for this test run.
	// +optional
	VictoriaLogs *VictoriaLogsConfig `json:"victoriaLogs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// TestRun is the Schema for the testruns API
type TestRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of TestRun
	// +required
	Spec TestRunSpec `json:"spec"`

	// status defines the observed state of TestRun
	// +optional
	Status TestRunStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// TestRunList contains a list of TestRun
type TestRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestRun{}, &TestRunList{})
}
