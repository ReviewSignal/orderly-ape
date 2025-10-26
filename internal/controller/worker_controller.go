// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	apereviewsignalcomv1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

// WorkerReconciler reconciles a Worker object
type WorkerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Worker object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *WorkerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apereviewsignalcomv1.Worker{}).
		Named("worker").
		Complete(r)
}
