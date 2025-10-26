// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

var (
	WorkerConditionReady = "Ready"
)

// WorkerSpec defines the desired state of Worker.
type WorkerSpec struct {
	KubeconfigSecret *corev1.SecretReference `json:"kubeconfigSecret,omitempty"`

	// Namespace where the worker will operate.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace,omitempty"`

	// NodeSelector to specify the nodes where the worker pods should be scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations to specify the tolerations for the worker pods.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// WorkerStatus defines the observed state of Worker.
type WorkerStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Worker is the Schema for the workers API.
type Worker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkerSpec   `json:"spec,omitempty"`
	Status WorkerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkerList contains a list of Worker.
type WorkerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Worker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Worker{}, &WorkerList{})
}
