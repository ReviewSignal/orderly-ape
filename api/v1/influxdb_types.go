// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// InfluxDBConditionReady means that the server is ready to accept requests.
	InfluxDBConditionReady = "Ready"

	// InfluxDBReasonReady indicates that the InfluxDB server is ready to accept requests.
	InfluxDBReasonReady = "InfluxDBReady"

	// InfluxDBReasonConnectionFailed indicates that the operator failed to connect to the InfluxDB server.
	InfluxDBReasonConnectionFailed = "ConnectionFailed"

	// InfluxDBReasonListBucketsFailed indicates that the operator failed to list buckets in the InfluxDB server.
	InfluxDBReasonListBucketsFailed = "ListBucketsFailed"

	// InfluxDBReasonGetOrgIDFailed indicates that the operator failed to get the organization ID from the InfluxDB server.
	InfluxDBReasonGetOrgIDFailed = "GetOrgIDFailed"

	// InfluxDBAnnotationGrafanaDatasource is the annotation key for the Grafana datasource name
	InfluxDBAnnotationGrafanaDatasource = "ape.reviewsignal.com/grafana-datasource"
	// InfluxDBAnnotationGrafanaDatasourceUID is the annotation key for the Grafana datasource UID
	InfluxDBAnnotationGrafanaDatasourceUID = "ape.reviewsignal.com/grafana-datasource-uid"
	// InfluxDBAnnotationGrafanaReadTokenID is the annotation key for the InfluxDB read-only token ID
	InfluxDBAnnotationGrafanaReadTokenID = "ape.reviewsignal.com/grafana-read-token-id"
)

// InfluxDBSpec defines the desired state of InfluxDB
type InfluxDBSpec struct {
	// Address is the address of the InfluxDB server, and must be accesible the from cluster,
	// as well as from the Worker clusters.
	// It can be either a hostname or an IP address, and can include port
	// +kubebuilder:validation:Required
	Address SourcedValue `json:"address"`

	// Token is the management token for InfluxDB.
	// It's main purpose is to create tokens for each test run.
	// +kubebuilder:validation:Required
	Token SourcedValue `json:"token"`

	// Organization is the organization name in InfluxDB for which
	// tokens will be created.
	Organization SourcedValue `json:"organization"`
}

// InfluxDBStatus defines the observed state of InfluxDB.
type InfluxDBStatus struct {
	// Version is the InfluxDB server version
	// +optional
	Version string `json:"version,omitempty"`

	// ServerStartTime is the time when the InfluxDB server was last started (uptime)
	// +optional
	ServerStartTime *metav1.Time `json:"serverStartTime,omitempty"`

	// OrganizationID is the ID of the organization in InfluxDB
	// +optional
	OrganizationID string `json:"organizationID,omitempty"`

	// Buckets is a list of existing buckets in the InfluxDB server
	// for the specified organization.
	// +optional
	Buckets []string `json:"buckets,omitempty"`

	// conditions represent the current state of the InfluxDB resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// InfluxDB is the Schema for the influxdbs API
type InfluxDB struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of InfluxDB
	// +required
	Spec InfluxDBSpec `json:"spec"`

	// status defines the observed state of InfluxDB
	// +optional
	Status InfluxDBStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// InfluxDBList contains a list of InfluxDB
type InfluxDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InfluxDB `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InfluxDB{}, &InfluxDBList{})
}
