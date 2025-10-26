// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type TestGitRepoSource struct {
	// Repository is the URL of the git repository containing the test scenario
	// For example:
	// - https://github.com/ReviewSignal/loadtesting
	//
	// We currently only support public repositories via http.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=10
	// +kubebuilder:validation:Pattern=`^(https?|git)://[^\s/$.?#].[^\s]*$`
	Repository string `json:"repository,omitempty"`

	// Revision is the git revision (branch, tag, commit) to checkout
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9._/-]+$`
	// +kubebuilder:default="main"
	Revision string `json:"revision,omitempty"`

	// Path is the path within the repository to the test scenario script
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default="loadtest.js"
	Path string `json:"path,omitempty"`
}

func (r *TestGitRepoSource) String() string {
	if r == nil {
		return "<nil>"
	}
	return r.Repository + "@" + r.Revision
}

// TestSource defines the source of a test scenario
// +kubebuilder:validation:XValidation:rule="((has(self.git) ? 1 : 0) + (has(self.script) ? 1 : 0)) < 2",message="Only one of 'git' or 'script' must be specified"
type TestSource struct {
	GitRepo *TestGitRepoSource `json:"git,omitempty"`

	// Script is the URL of the test scenario script
	Script *string `json:"script,omitempty"`
}

// TestScenarioSpec defines the desired state of TestScenario
type TestScenarioSpec struct {
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

	// Workers is a list of references to Workers (locations) where the test should run.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	Workers []TestRunWorkerSpec `json:"workers,omitempty"`

	// EnvVars is a list of environment variables to set for the test run.
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=24
	// +optional
	EnvVars []EnvVar `json:"envVars,omitempty"`

	// Labels are test results labels to set for the test run.
	// +kubebuilder:validation:MaxProperties=24
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// TestScenarioStatus defines the observed state of TestScenario.
type TestScenarioStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// TestScenario is the Schema for the testscenarios API
type TestScenario struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of TestScenario
	// +required
	Spec TestScenarioSpec `json:"spec"`

	// status defines the observed state of TestScenario
	// +optional
	Status TestScenarioStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// TestScenarioList contains a list of TestScenario
type TestScenarioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestScenario `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestScenario{}, &TestScenarioList{})
}
