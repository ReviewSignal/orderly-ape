// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ObjectReference represents an Object Reference. It has enough information to retrieve an object
// from a predetermined group, version and kind in any namespace
// +structType=atomic
type ObjectReference struct {
	// name is unique within a namespace to reference an object resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// namespace defines the space within which the object name must be unique.
	Namespace string `json:"namespace"`
}

func (o *ObjectReference) AsNamespacedName() types.NamespacedName {
	if o == nil {
		return types.NamespacedName{}
	}
	return types.NamespacedName{
		Name:      o.Name,
		Namespace: o.Namespace,
	}
}

// SourcedValue can either be a string or a reference to a ConfigMap or Secret key.
type SourcedValue struct {
	Value     string     `json:"value,omitempty"`
	ValueFrom *ValueFrom `json:"valueFrom,omitempty"`
}

type ValueFrom struct {
	ConfigMapKeyRef *corev1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	SecretKeyRef    *corev1.SecretKeySelector    `json:"secretKeyRef,omitempty"`
}
