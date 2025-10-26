// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const OrderlyApeManager = "ape.reviewsignal.com"

const LabelPrefix = "ape.reviewsignal.com/"
const AnnotationPrefix = LabelPrefix
const FinalizerPrefix = LabelPrefix

const DisplayNameAnnotation = AnnotationPrefix + "display-name"
const DescriptionAnnotation = AnnotationPrefix + "description"
const AutoApplyTolerationsAnnotation = AnnotationPrefix + "auto-apply-tolerations"

const SyncAnnotation = AnnotationPrefix + "sync"
const SyncedFromAnnotation = AnnotationPrefix + "synced-from"

const DraftAnnotation = AnnotationPrefix + "draft"

const OrderlyApeProjectNamespace = AnnotationPrefix + "project"

const MySQLUserFinalizer = FinalizerPrefix + "created-in-mysql"
const MySQLSchemaFinalizer = FinalizerPrefix + "created-in-mysql"
const CertificateFinalizer = FinalizerPrefix + "certificate"
const WordPressCertMapFinalizer = FinalizerPrefix + "certmap"

const KubernetesAppInstance = "app.kubernetes.io/instance"
const KubernetesAppName = "app.kubernetes.io/name"
const KubernetesAppPartOf = "app.kubernetes.io/part-of"
const KubernetesAppComponent = "app.kubernetes.io/component"
const KubernetesAppManagedBy = "app.kubernetes.io/managed-by"
const KubernetesAppVersion = "app.kubernetes.io/version"

func DisplayName(o metav1.Object) string {
	annotations := o.GetAnnotations()
	if annotations == nil {
		return o.GetName()
	}
	if name, ok := annotations[DisplayNameAnnotation]; ok {
		return name
	}
	return o.GetName()
}

func Description(o metav1.Object) string {
	annotations := o.GetAnnotations()
	if annotations == nil {
		return ""
	}
	if desc, ok := annotations[DescriptionAnnotation]; ok {
		return desc
	}
	return ""
}

func Annotation(name string) string {
	return AnnotationPrefix + name
}

func Label(name string) string {
	return LabelPrefix + name
}

func Finalizer(name string) string {
	return FinalizerPrefix + name
}
