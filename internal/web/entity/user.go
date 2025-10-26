// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"github.com/a-h/templ"
	dexapi "github.com/dexidp/dex/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
)

// user wraps dexapi.Password to implement metav1.Object and runtime.Object interfaces
type user struct {
	dexapi.Password
}

type User struct {
	*user
}

var _ datatable.Entry = User{}

type UserList []User

func WrapDexPassword(password *dexapi.Password) User {
	hashCopy := make([]byte, len(password.Hash))
	copy(hashCopy, password.Hash)
	u := &user{
		Password: dexapi.Password{
			Email:    password.GetEmail(),
			Hash:     hashCopy,
			Username: password.GetUsername(),
			UserId:   password.GetUserId(),
		},
	}
	return User{user: u}
}

func (e User) NamespacedName() string {
	// DEX users don't have a namespace, so return empty namespace with email as name
	return "/" + e.Email
}

func (e User) ObjectLink() templ.SafeURL {
	return templ.URL("")
}

func (e User) VerboseName() string {
	return "User"
}

// GetDisplayName returns the user's display name
func (e User) GetDisplayName() string {
	// For DEX users, use username or email
	if e.user != nil {
		if e.Username != "" {
			return e.Username
		}
		return e.Email
	}
	return ""
}

// Implement metav1.Object and runtime.Object interfaces
func (e *user) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (e *user) DeepCopyObject() runtime.Object {
	if e == nil {
		return nil
	}
	// Deep copy the password hash
	hashCopy := make([]byte, len(e.Hash))
	copy(hashCopy, e.Hash)
	return &user{
		Password: dexapi.Password{
			Email:    e.Email,
			Hash:     hashCopy,
			Username: e.Username,
			UserId:   e.UserId,
		},
	}
}

func (e *user) GetNamespace() string {
	return ""
}

func (e *user) SetNamespace(namespace string) {
}

func (e *user) GetName() string {
	return e.Email
}

func (e *user) SetName(name string) {
}

func (e *user) GetGenerateName() string {
	return ""
}

func (e *user) SetGenerateName(name string) {
}

func (e *user) GetUID() types.UID {
	return types.UID(e.UserId)
}

func (e *user) SetUID(uid types.UID) {
}

func (e *user) GetResourceVersion() string {
	return ""
}

func (e *user) SetResourceVersion(version string) {
}

func (e *user) GetGeneration() int64 {
	return 0
}

func (e *user) SetGeneration(generation int64) {
}

func (e *user) GetSelfLink() string {
	return ""
}

func (e *user) SetSelfLink(selfLink string) {
}

func (e *user) GetCreationTimestamp() metav1.Time {
	return metav1.Time{}
}

func (e *user) SetCreationTimestamp(timestamp metav1.Time) {
}

func (e *user) GetDeletionTimestamp() *metav1.Time {
	return nil
}

func (e *user) SetDeletionTimestamp(timestamp *metav1.Time) {
}

func (e *user) GetDeletionGracePeriodSeconds() *int64 {
	return nil
}

func (e *user) SetDeletionGracePeriodSeconds(*int64) {
}

func (e *user) GetLabels() map[string]string {
	return nil
}

func (e *user) SetLabels(labels map[string]string) {
}

func (e *user) GetAnnotations() map[string]string {
	return nil
}

func (e *user) SetAnnotations(annotations map[string]string) {
}

func (e *user) GetFinalizers() []string {
	return nil
}

func (e *user) SetFinalizers(finalizers []string) {
}

func (e *user) GetOwnerReferences() []metav1.OwnerReference {
	return nil
}

func (e *user) SetOwnerReferences([]metav1.OwnerReference) {
}

func (e *user) GetManagedFields() []metav1.ManagedFieldsEntry {
	return nil
}

func (e *user) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {
}
