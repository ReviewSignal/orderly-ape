// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	mctrl "sigs.k8s.io/multicluster-runtime"
	mbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcontext "sigs.k8s.io/multicluster-runtime/pkg/context"
	mhandler "sigs.k8s.io/multicluster-runtime/pkg/handler"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/jwt"
)

const requeueInterval = 10 * time.Second

// TestRunWorkerReconciler reconciles a TestScenario object
type TestRunWorkerReconciler struct {
	client.Client
	Scheme                    *runtime.Scheme
	LocalNamespace            string
	GetCluster                func(ctx context.Context, name string) (cluster.Cluster, error)
	VictoriaLogsInsertURL     string
	ProxiedVictoriaLogsInsert bool

	JWTManager *jwt.Manager
	mu         sync.Mutex
	igniters   Igniters
}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testscenarios,verbs=get;list;watch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=influxdbs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Takes the worker through its phases.
// These are TestRun phase transitions. They are global phases (ie. `TestRun.Phase()`),
// not individual worker phases.
//
// Pending -> Queued -> Initializing -> Ready -> Running -> Completed/Failed/Canceled
// Pending - The worker has accepted the TestRun but has not started it yet.
// Pending -> Queued - The worker creates the TestRun Job and auxiliary resources.
// Queued -> Ready - The worker has started the Job and it's waiting for its pods to be stably ready (ie. up for X seconds)
// Ready -> Running - Worker creates the TestRun Job igniter, that starts the actual test run, and set the phase to Running
// Running -> Completed/Failed/Canceled - The worker monitors the Job and updates the phase accordingly.
//
// * -> Failed - At least one worker has failed. The current worker if it's not already in a terminal phase
// (Completed, Canceled, Failed) is Canceled.
//
// * -> Canceled - The TestRun has been canceled. The current worker if it's not already in a terminal phase
// (Completed, Canceled, Failed) is Canceled.
//
//nolint:gocyclo // Complex reconciliation logic requires this complexity
func (r *TestRunWorkerReconciler) Reconcile(ctx context.Context, req mctrl.Request) (mctrl.Result, error) {
	var err error
	l := logf.FromContext(ctx).WithValues("testrun", req.NamespacedName, "worker", req.ClusterName)
	ctx = mcontext.WithCluster(ctx, req.ClusterName)
	l.Info("Reconciling TestRunWorker")

	workerFullName := req.ClusterName
	parts := strings.SplitN(workerFullName, "/", 2)
	if len(parts) != 2 {
		return mctrl.Result{}, fmt.Errorf("invalid worker cluster name: %s", workerFullName)
	}
	workerName := parts[1]
	cluster, err := r.GetCluster(ctx, workerFullName)
	if err != nil {
		return mctrl.Result{}, fmt.Errorf("failed to get worker cluster: %w", err)
	}
	cl := cluster.GetClient()

	// Fetch the TestRun instance
	testrun := &apev1.TestRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: r.LocalNamespace,
		},
	}
	if err = r.Get(ctx, client.ObjectKeyFromObject(testrun), testrun); client.IgnoreNotFound(err) != nil {
		return mctrl.Result{}, err
	}

	if testrun.DeletionTimestamp.IsZero() && (isDraft(testrun) || isArchived(testrun) || isCancelCompleted(testrun)) {
		return mctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) || !testrun.DeletionTimestamp.IsZero() {
		err = cl.Delete(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
		}, client.PropagationPolicy(metav1.DeletePropagationBackground))
		return mctrl.Result{}, client.IgnoreNotFound(err)
	}

	workerSpec := workerSpecByName(workerName, testrun.Spec.Workers)
	if workerSpec == nil {
		return mctrl.Result{}, fmt.Errorf("worker '%s' not found in TestRun spec", workerName)
	}
	status := r.getWorkerStatus(testrun, workerName)

	if status.Phase == apev1.TestRunPhaseCompleted || status.Phase == apev1.TestRunPhaseFailed {
		r.removeIgniter(testrun, workerName)
		return mctrl.Result{}, nil
	}

	job := &batchv1.Job{}
	if err = cl.Get(ctx, req.NamespacedName, job); client.IgnoreNotFound(err) != nil {
		return mctrl.Result{}, err
	}
	if !job.DeletionTimestamp.IsZero() {
		return mctrl.Result{}, nil
	}
	jobFound := !apierrors.IsNotFound(err)

	phase := testrun.Phase()
	l = l.WithValues("state", testrun.Spec.State, "phase", phase)

	// If the job is canceled, we need to suspend the job
	if testrun.Spec.State == apev1.TestRunStateCanceled {
		statusUpdate := r.setWorkerPhase(testrun, workerName, apev1.TestRunPhaseCanceled, metav1.Condition{
			Type:    apev1.TestRunWorkerConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  apev1.TestRunWorkerReasonCanceled,
			Message: "TestRun has been canceled",
		})
		// If the Job exists in kubernetes, we need to suspend it
		if err == nil && (job.Spec.Suspend == nil || !*job.Spec.Suspend) {
			l.Info("Suspending canceled TestRun")
			job.Spec.Suspend = truePtr
			err = cl.Update(ctx, job)
		}
		r.removeIgniter(testrun, workerName)
		if client.IgnoreNotFound(err) != nil {
			statusUpdate = statusUpdate || r.setWorkerPhase(testrun, workerName, apev1.TestRunPhaseCanceled, metav1.Condition{
				Type:    apev1.TestRunWorkerConditionFailed,
				Status:  metav1.ConditionTrue,
				Reason:  apev1.TestRunWorkerReasonCancelFailed,
				Message: err.Error(),
			})
		}
		var updateErr error
		if statusUpdate {
			updateErr = r.Status().Update(ctx, testrun)
		}
		return mctrl.Result{}, errors.Join(client.IgnoreNotFound(err), updateErr)
	}

	if !jobFound && status.Phase != apev1.TestRunPhaseUnknown && status.Phase != apev1.TestRunPhasePending {
		return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseFailed, metav1.Condition{
			Type:    apev1.TestRunWorkerConditionFailed,
			Status:  metav1.ConditionTrue,
			Reason:  apev1.TestRunWorkerReasonJobNotFound,
			Message: fmt.Sprintf("Test was `%s` but no Kubernetes Job found", status.Phase),
		})
	}

	// From here on, we are handling jobs that are not completed, failed or canceled
	l.Info("Reconciling TestRun")

	if status.Phase == apev1.TestRunPhasePending {
		if !jobFound {
			l.V(1).Info("Creating Job for TestRun worker")

			if testrun.Status.InfluxDBSecret == nil {
				return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseFailed, metav1.Condition{
					Type:    apev1.TestRunWorkerConditionFailed,
					Status:  metav1.ConditionTrue,
					Reason:  apev1.TestRunWorkerReasonJobCreateFailed,
					Message: "influxdb secret not found in testrun .status.influxdbSecret",
				})
			}
			worker, err := r.getWorker(ctx, workerFullName)
			if err != nil {
				return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseFailed, metav1.Condition{
					Type:    apev1.TestRunWorkerConditionFailed,
					Status:  metav1.ConditionTrue,
					Reason:  apev1.TestRunWorkerReasonJobCreateFailed,
					Message: fmt.Sprintf("failed to get worker '%s': %v", workerName, err),
				})
			}

			job, err = r.syncJob(ctx, worker, testrun)
			if err != nil {
				return mctrl.Result{}, errors.Join(err, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseFailed, metav1.Condition{
					Type:    apev1.TestRunWorkerConditionFailed,
					Status:  metav1.ConditionTrue,
					Reason:  apev1.TestRunWorkerReasonJobCreateFailed,
					Message: fmt.Sprintf("Worker pods have failed running k6 tests: %s", err),
				}))
			}

			l.V(1).Info("Creating telegraf config for TestRun")
			influxDBCredentials := &corev1.Secret{}
			err = r.Get(ctx, testrun.Status.InfluxDBSecret.AsNamespacedName(), influxDBCredentials)
			if err != nil {
				return mctrl.Result{}, fmt.Errorf("failed to get influxdb credentials secret: %w", err)
			}

			_, err = r.syncTelegrafConfig(ctx, worker, testrun, job, influxDBCredentials)
			if err != nil {
				return mctrl.Result{}, err
			}

			l.V(1).Info("Creating PDB for TestRun")
			_, err = r.syncPodDisruptionBudget(ctx, worker, testrun, job)
			if err != nil {
				return mctrl.Result{}, err
			}
		}

		l.V(1).Info("Changing TestRun worker phase to Queued")
		return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseQueued, metav1.Condition{
			Type:    apev1.TestRunWorkerConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  apev1.TestRunWorkerReasonJobCreated,
			Message: "Test run is queued for execution",
		})
	}

	if cond := getJobCondition(job, batchv1.JobFailed); cond != nil {
		return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseFailed, metav1.Condition{
			Type:    apev1.TestRunWorkerConditionFailed,
			Status:  metav1.ConditionTrue,
			Reason:  apev1.TestRunWorkerReasonJobFailed,
			Message: fmt.Sprintf("Worker pods have failed running k6 tests: %s", cond.Message),
		})
	}

	if status.Phase == apev1.TestRunPhaseQueued {
		if job.Status.Ready != nil && int32(len(status.AssignedSegments)) == *job.Status.Ready {
			pods, err := r.getPods(ctx, job)
			if err != nil {
				return mctrl.Result{}, err
			}
			ready := true
			for _, pod := range pods {
				if !isPodStableReady(&pod) {
					ready = false
				}
			}
			if ready {
				return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseReady, metav1.Condition{
					Type:    apev1.TestRunWorkerConditionReady,
					Status:  metav1.ConditionTrue,
					Reason:  apev1.TestRunWorkerReasonPodsReady,
					Message: "All worker pods are stable and ready",
				})
			}
		}
		l.V(1).Info("Waiting for pods to be stable-ready")
	}

	if status.Phase == apev1.TestRunPhaseReady {
		igniter, err := r.createIgniter(ctx, testrun, job)
		if err != nil {
			return mctrl.Result{}, err
		}
		l.V(1).Info("Waiting for job to be ignited")

		igniterStarted := igniter.Started()
		igniterError := igniter.Error()

		if igniterStarted && igniterError == nil {
			return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseRunning)
		}

		if igniterError != nil {
			return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseFailed, metav1.Condition{
				Type:    apev1.TestRunWorkerConditionFailed,
				Status:  metav1.ConditionTrue,
				Reason:  apev1.TestRunWorkerReasonIgniterFailed,
				Message: fmt.Sprintf("Worker pods have failed running k6 tests: %s", igniterError),
			})
		}
	}

	if status.Phase == apev1.TestRunPhaseRunning {
		l.V(1).Info("TestRun is Running, checking for completion")
		if job.Status.Active == 0 {
			if int(job.Status.Succeeded) == len(status.AssignedSegments) {
				return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseCompleted)
			} else {
				return mctrl.Result{}, r.updateWorkerPhase(ctx, testrun, workerName, apev1.TestRunPhaseFailed, metav1.Condition{
					Type:    apev1.TestRunWorkerConditionFailed,
					Status:  metav1.ConditionTrue,
					Reason:  apev1.TestRunWorkerReasonJobFailed,
					Message: "Worker pods have failed running k6 tests",
				})
			}
		}
	}

	return mctrl.Result{RequeueAfter: requeueInterval}, nil
}

// getWorkerStatus should return a copy, not a pointer to slice element
func (r *TestRunWorkerReconciler) getWorkerStatus(testrun *apev1.TestRun, worker string) apev1.TestRunWorkerStatus {
	for _, st := range testrun.Status.WorkerStatuses {
		if st.Name == worker {
			return st
		}
	}
	return apev1.TestRunWorkerStatus{Name: worker, Phase: apev1.TestRunPhaseUnknown}
}

//nolint:unparam // Bool return used by caller to determine if update is needed
func (r *TestRunWorkerReconciler) setWorkerPhase(testrun *apev1.TestRun, workerName string, phase apev1.TestRunPhase, conditions ...metav1.Condition) bool {
	// Find the status (returns copy, not pointer)
	var statusIdx = -1
	for i := range testrun.Status.WorkerStatuses {
		if testrun.Status.WorkerStatuses[i].Name == workerName {
			statusIdx = i
			break
		}
	}

	if statusIdx == -1 {
		// Worker status doesn't exist, create it
		testrun.Status.WorkerStatuses = append(testrun.Status.WorkerStatuses, apev1.TestRunWorkerStatus{
			Name:       workerName,
			Phase:      phase,
			Conditions: []metav1.Condition{},
		})
		return true
	}

	status := &testrun.Status.WorkerStatuses[statusIdx]

	// No need to change, already in desired phase
	if status.Phase == phase {
		updated := false
		for _, condition := range conditions {
			updated = updated || meta.SetStatusCondition(&status.Conditions, condition)
		}
		return updated
	}

	// Update the phase
	status.Phase = phase
	for _, condition := range conditions {
		meta.SetStatusCondition(&status.Conditions, condition)
	}

	return true
}

func (r *TestRunWorkerReconciler) updateWorkerPhase(ctx context.Context, testrun *apev1.TestRun, workerName string, phase apev1.TestRunPhase, conditions ...metav1.Condition) error {
	changed := r.setWorkerPhase(testrun, workerName, phase, conditions...)
	if !changed {
		return nil
	}

	// Retry on conflict errors
	err := r.Status().Update(ctx, testrun)
	if apierrors.IsConflict(err) {
		// Refetch the testrun and retry once
		fresh := &apev1.TestRun{}
		if getErr := r.Get(ctx, client.ObjectKeyFromObject(testrun), fresh); getErr != nil {
			return getErr
		}
		// Apply the phase change to the fresh object
		r.setWorkerPhase(fresh, workerName, phase, conditions...)
		return r.Status().Update(ctx, fresh)
	}
	return err
}

func (r *TestRunWorkerReconciler) clientFromContext(ctx context.Context) (client.Client, error) {
	clusterName, ok := mcontext.ClusterFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("cluster not found in context")
	}
	cluster, err := r.GetCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	return cluster.GetClient(), nil
}

func (r *TestRunWorkerReconciler) getWorker(ctx context.Context, clusterName string) (*apev1.Worker, error) {
	parts := strings.SplitN(clusterName, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cluster name format: %s", clusterName)
	}
	worker := &apev1.Worker{}
	if err := r.Get(ctx, types.NamespacedName{Name: parts[1], Namespace: parts[0]}, worker); err != nil {
		return nil, err
	}

	return worker, nil
}

func (r *TestRunWorkerReconciler) getPods(ctx context.Context, job *batchv1.Job) ([]corev1.Pod, error) {
	cl, err := r.clientFromContext(ctx)
	if err != nil {
		return nil, err
	}

	pods := &corev1.PodList{}
	err = cl.List(ctx, pods, client.InNamespace(job.GetNamespace()), client.MatchingLabels{
		"batch.kubernetes.io/job-name": job.GetName(),
	})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (r *TestRunWorkerReconciler) mapTestRunToWorkers(ctx context.Context, o client.Object) []mctrl.Request {
	l := logf.FromContext(ctx).WithValues("testrun", client.ObjectKeyFromObject(o))
	testrun, ok := o.(*apev1.TestRun)
	if !ok {
		l.Error(nil, "object is not a TestRun")
		return nil
	}

	workers := &apev1.WorkerList{}
	if err := r.List(ctx, workers); err != nil {
		l.Error(err, "failed to list workers")
		return nil
	}

	if testrun.DeletionTimestamp.IsZero() && (isDraft(testrun) || isArchived(testrun) || isCancelCompleted(testrun)) {
		l.V(1).Info("Skipping mapping TestRun to workers, TestRun is draft, archived or cancel completed")
		return nil
	}

	reqs := make([]mctrl.Request, 0, len(testrun.Spec.Workers))
	for _, worker := range testrun.Spec.Workers {
		workerFullName := r.LocalNamespace + "/" + worker.Name
		w, err := r.getWorker(ctx, workerFullName)
		if err != nil {
			l.Error(err, "failed to get worker", "worker", worker.Name)
			return nil
		}
		reqs = append(reqs, mctrl.Request{
			Request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      testrun.Name,
					Namespace: w.Spec.Namespace,
				},
			},
			ClusterName: workerFullName,
		})
	}

	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunWorkerReconciler) SetupWithManager(mgr mctrl.Manager) error {
	r.GetCluster = mgr.GetCluster
	r.igniters = Igniters{}
	return mctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
		Watches(
			&apev1.TestRun{},
			mhandler.TypedEventHandlerFunc[client.Object, mctrl.Request](
				func(clusterName string, cl cluster.Cluster) handler.TypedEventHandler[client.Object, mctrl.Request] {
					return handler.TypedEnqueueRequestsFromMapFunc(handler.TypedMapFunc[client.Object, mctrl.Request](r.mapTestRunToWorkers))
				},
			),
			mbuilder.WithEngageWithLocalCluster(true),
			mbuilder.WithEngageWithProviderClusters(false),
		).
		Named("testrun-worker").
		Complete(r)
}
