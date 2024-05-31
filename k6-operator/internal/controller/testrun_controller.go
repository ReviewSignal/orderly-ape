//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2024 ReviewSignal

package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1util "kmodules.xyz/client-go/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	loadtestingv1alpha1 "github.com/ReviewSignal/loadtesting/k6-operator/api/v1alpha1"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting"
	loadtestingapi "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/api"
)

var (
	zero int64 = 0
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
	l.Info("Reconciling TestRun", "name", req.NamespacedName)

	obj := &loadtestingv1alpha1.TestRun{}
	err := r.Get(ctx, req.NamespacedName, obj)

	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		obj, err = r.createTestRun(ctx, req)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	for idx := range obj.Spec.AssignedSegments {
		segment := obj.Spec.AssignedSegments[idx]
		err = r.syncPod(ctx, obj, idx, segment)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) syncPod(ctx context.Context, owner *loadtestingv1alpha1.TestRun, idx int, segment string) error {
	obj := &corev1.Pod{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", owner.Name, idx),
			Namespace: owner.Namespace,
		},
	}
	_, err := ctrl.CreateOrUpdate(ctx, r.Client, obj, func() error {
		err := util.SetControllerReference(owner, obj, r.Scheme)
		if err != nil {
			return err
		}

		if len(obj.Labels) == 0 {
			obj.Labels = make(map[string]string)
		}
		obj.Labels["app.kubernetes.io/name"] = "k6"
		obj.Labels["app.kubernetes.io/instance"] = owner.Name
		obj.Labels["app.kubernetes.io/managed-by"] = "reviewsignal-k6-operator"

		obj.Spec.RestartPolicy = corev1.RestartPolicyNever
		obj.Spec.TerminationGracePeriodSeconds = &zero

		command := []string{"k6", "run", "--paused", "--address=0.0.0.0:6565", fmt.Sprintf("--tag=%s", owner.Name)}

		if len(owner.Spec.Segments) > 1 {
			command = append(command, fmt.Sprintf("--execution-segment-sequence=%s", strings.Join(owner.Spec.Segments, ",")))
			command = append(command, fmt.Sprintf("--execution-segment=%s", segment))
		}

		command = append(command, owner.Spec.SourceScript)

		volumes := obj.Spec.Volumes
		if corev1util.GetVolumeByName(volumes, "k6-script") == nil {
			obj.Spec.Volumes = corev1util.UpsertVolume(obj.Spec.Volumes,
				corev1.Volume{
					Name: "k6-script",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			)
		}

		initContainers := obj.Spec.InitContainers
		if corev1util.GetContainerByName(initContainers, "git") == nil {
			obj.Spec.InitContainers = corev1util.UpsertContainer(obj.Spec.InitContainers,
				corev1.Container{
					Name:            "git",
					Image:           "alpine/git",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command: []string{"/bin/sh", "-c",
						strings.Join([]string{
							"cd /scripts",
							"set -eo pipefail",
							"set -x",
							"git init",
							"git remote add origin https://" + owner.Spec.SourceRepo,
							"git fetch --depth=1 origin " + owner.Spec.SourceRef,
						}, "\n"),
					},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "k6-script",
						MountPath: "/scripts",
					}},
				})
		}

		containers := obj.Spec.Containers
		if corev1util.GetContainerByName(containers, "k6") == nil {
			obj.Spec.Containers = corev1util.UpsertContainer(obj.Spec.Containers,
				corev1.Container{
					Name:            "k6",
					Image:           "loadimpact/k6",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         command,
					WorkingDir:      "/scripts",
					Ports: []corev1.ContainerPort{{
						Name:          "http-api",
						ContainerPort: 6565,
					}},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "k6-script",
						MountPath: "/scripts",
					}},
				},
			)
		}

		return nil
	})

	return err
}

func (r *TestRunReconciler) createTestRun(ctx context.Context, req ctrl.Request) (*loadtestingv1alpha1.TestRun, error) {
	l := log.FromContext(ctx)
	job := &loadtestingapi.Job{}
	err := r.APIClient.Get(ctx, req.Name, job)
	if err != nil {
		l.Error(err, "Failed retrieving Job from API", "job", job)
		return nil, err
	}

	obj, ok := job.ToK8SResource().(*loadtestingv1alpha1.TestRun)
	if !ok {
		return nil, fmt.Errorf("failed to convert job to TestRun")
	}

	err = r.Create(ctx, obj)

	return obj, err
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
