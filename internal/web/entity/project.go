// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"net/url"

	"github.com/a-h/templ"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Project struct {
	*corev1.Namespace
}

type ProjectList []Project

func WrapNamepace(ns *corev1.Namespace) Project {
	return Project{ns}
}

func WrapNamespaceList(items []corev1.Namespace) ProjectList {
	wrapped := make(ProjectList, len(items))
	for i, item := range items {
		wrapped[i] = WrapNamepace(&item)
	}
	return wrapped
}

func (p Project) NamespacedName() string {
	return client.ObjectKeyFromObject(p.Namespace).String()
}

func (p Project) ObjectLink() templ.SafeURL {
	path, _ := url.JoinPath("projects", p.Name)
	return templ.URL(path)
}

func (p Project) VerboseName() string {
	return "Project"
}
