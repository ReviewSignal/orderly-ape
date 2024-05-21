//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2024 ReviewSignal

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	loadtestingv1alpha1 "github.com/ReviewSignal/loadtesting/k6-operator/api/v1alpha1"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting"
	loadtestingapi "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/api"
)

// TestRunReconciler reconciles a TestRun object
type TestRunReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	APIClient loadtesting.Client
}

//+kubebuilder:rbac:groups=loadtesting.reviewsignal.org,resources=testruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=loadtesting.reviewsignal.org,resources=testruns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=loadtesting.reviewsignal.org,resources=testruns/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the TestRun object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *TestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	l.Info("Reconciling TestRun")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	worker, err := loadtesting.NewWorker(r.APIClient, &loadtestingapi.Job{})
	if err != nil {
		return err
	}

	if err = mgr.Add(worker); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&loadtestingv1alpha1.TestRun{}).
		WatchesRawSource(&source.Channel{Source: worker.C}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
			return []ctrl.Request{
				{NamespacedName: types.NamespacedName{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
				}},
			}
		})).
		Complete(r)
}
