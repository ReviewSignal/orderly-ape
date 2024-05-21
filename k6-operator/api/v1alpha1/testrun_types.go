//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2024 ReviewSignal

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestRunSpec defines the desired state of TestRun
type TestRunSpec struct {
	Target           string   `json:"target"`
	SourceRepo       string   `json:"sourceRepo"`
	SourceRef        string   `json:"sourceRef"`
	SourceScript     string   `json:"sourceScript"`
	Workers          int32    `json:"workers"`
	Segments         []string `json:"segments"`
	AssignedSegments []string `json:"assignedSegments"`
}

// TestRunStatus defines the observed state of TestRun
type TestRunStatus struct {
	Status        string `json:"status"`
	Description   string `json:"description"`
	OnlineWorkers int32  `json:"onlineWorkers"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// TestRun is the Schema for the testruns API
type TestRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestRunSpec   `json:"spec,omitempty"`
	Status TestRunStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TestRunList contains a list of TestRun
type TestRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestRun{}, &TestRunList{})
}
