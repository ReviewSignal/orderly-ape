// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

// WorkerIndexer reconciles a Secret object
type WorkerIndexer struct {
	client.Client
	Scheme *runtime.Scheme
	client.FieldIndexer
}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers/finalizers,verbs=update

func (i *WorkerIndexer) IndexKubeconfigSecret(ctx context.Context) error {
	return i.IndexField(ctx, &apev1.Worker{}, "spec.kubeconfigSecret", func(o client.Object) []string {
		worker := o.(*apev1.Worker)
		if worker.Spec.KubeconfigSecret == nil {
			return nil
		}

		namespace := worker.Spec.KubeconfigSecret.Namespace
		if namespace == "" {
			namespace = worker.Namespace
		}
		namespacedName := namespace + "/" + worker.Spec.KubeconfigSecret.Name
		return []string{namespacedName}
	})
}

// SetupWithManager sets up the indexer with the Manager.
func (i *WorkerIndexer) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		err := i.IndexKubeconfigSecret(ctx)
		if err != nil {
			return err
		}

		return nil
	}))
}
