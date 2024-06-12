//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2024 ReviewSignal

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alessio/shellescape"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
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
	loadtestingruntime "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

var (
	zero     int64 = 0
	truePtr  *bool = func(b bool) *bool { return &b }(true)
	falsePtr *bool = func(b bool) *bool { return &b }(false)
)

// TestRunReconciler reconciles a TestRun object
type TestRunReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	APIClient loadtesting.Client
	Location  string
	clientset clientset.Interface
	igniters  Igniters
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
	l := log.FromContext(ctx).WithValues("name", req.NamespacedName)

	job := &loadtestingapi.Job{}
	err := r.APIClient.Get(ctx, req.Name, job)
	if err != nil {
		l.Error(err, "Failed retrieving Job from API", "job", job)
		return ctrl.Result{}, err
	}

	l = l.WithValues("status", job.Status)
	l.Info("Reconciling TestRun")

	obj := &loadtestingv1alpha1.TestRun{}
	err = r.Get(ctx, req.NamespacedName, obj)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if obj.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) {
		obj, err = r.createTestRun(ctx, job)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if job.Status == loadtestingapi.STATUS_PENDING {
		job.Status = loadtestingapi.STATUS_QUEUED
		job.StatusDescription = "Test run is queued for execution"
		err = r.APIClient.Update(ctx, job)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if job.Status != loadtestingapi.STATUS_COMPLETED && job.Status != loadtestingapi.STATUS_FAILED {
		for idx := range obj.Spec.AssignedSegments {
			segment := obj.Spec.AssignedSegments[idx]
			err = r.syncPod(ctx, obj, idx, job, segment)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if job.Status == loadtestingapi.STATUS_QUEUED {
		var podsReady int32
		for idx := range obj.Spec.AssignedSegments {
			pod := &corev1.Pod{}
			err = r.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%d", obj.Name, idx), Namespace: obj.Namespace}, pod)
			if err == nil && isPodStableReady(pod) {
				podsReady++
			}
		}

		update := false
		if podsReady != job.OnlineWorkers {
			job.OnlineWorkers = podsReady
			update = true
		}
		if int(podsReady) == len(job.AssignedSegments) {
			job.Status = loadtestingapi.STATUS_READY
			job.StatusDescription = "Worker pods are ready and waiting to start testing"
			update = true
		}
		if update {
			err = r.APIClient.Update(ctx, job)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if job.TestRun.Ready {
		if job.TestRun.StartTestAt == nil {
			return ctrl.Result{}, fmt.Errorf("TestRun is ready but StartTestAt is nil")
		}

		igniter, err := r.createIgniter(ctx, job)
		if err != nil {
			return ctrl.Result{}, err
		}

		if job.Status == loadtestingapi.STATUS_READY && igniter.Started && igniter.Error == nil {
			job.Status = loadtestingapi.STATUS_RUNNING
			job.StatusDescription = "Worker pods are currently running k6 tests"
			err = r.APIClient.Update(ctx, job)
			if err != nil {
				l.Error(err, "Failed updating job status", "job", job)
			}
		}

		if igniter.Error != nil {
			job.Status = loadtestingapi.STATUS_FAILED
			err = r.APIClient.Update(ctx, job)
			if err != nil {
				l.Error(err, "Failed updating job status", "job", job)
			}
		}
	}

	if job.Status == loadtestingapi.STATUS_RUNNING {
		completed := true
		success := true

		for idx := range obj.Spec.AssignedSegments {
			pod := &corev1.Pod{}
			err = r.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%d", obj.Name, idx), Namespace: obj.Namespace}, pod)
			if err != nil {
				return ctrl.Result{}, err
			}

			completed = completed && isPodCompleted(pod)
			if completed {
				success = success && isPodSucceeded(pod)
			}
		}

		if completed {
			if success {
				job.Status = loadtestingapi.STATUS_COMPLETED
				job.StatusDescription = "Worker pods have successfully completed running k6 tests"
			} else {
				job.StatusDescription = "Worker pods have failed running k6 tests"
				job.Status = loadtestingapi.STATUS_FAILED
			}

			err = r.APIClient.Update(ctx, job)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) syncPod(ctx context.Context, owner *loadtestingv1alpha1.TestRun, idx int, job *loadtestingapi.Job, segment string) error {
	obj := &corev1.Pod{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", owner.Name, idx),
			Namespace: owner.Namespace,
		},
	}
	_, err := ctrl.CreateOrUpdate(ctx, r.Client, obj, func() error {
		err := util.SetOwnerReference(owner, obj, r.Scheme)
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

		obj.Spec.NodeSelector = job.TestRun.NodeSelector

		if job.TestRun.DedicatedNodes && obj.Spec.Affinity == nil {
			obj.Spec.Affinity = &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							TopologyKey: corev1.LabelHostname,
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name": "k6",
								},
							},
						},
					},
				},
			}
		}

		command := []string{"k6", "run", "--paused", "--address", "0.0.0.0:6565",
			"--tag", fmt.Sprintf("testid=%s", owner.Name),
			"--tag", fmt.Sprintf("location=%s", r.Location),
			"--tag", fmt.Sprintf("instance_id=%s-%d", r.Location, idx),
			"--tag", fmt.Sprintf("pod_name=%s", obj.Name),
		}

		if len(owner.Spec.Segments) > 1 {
			command = append(command, "--execution-segment-sequence", strings.Join(owner.Spec.Segments, ","))
			command = append(command, "--execution-segment", segment)
		}

		command = append(command, owner.Spec.SourceScript)

		script := shellescape.QuoteCommand(command)

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
					WorkingDir:      "/scripts",
					Command: []string{"/bin/sh", "-c",
						strings.Join([]string{
							"set -eo pipefail",
							"set -x",
							"git init",
							"git remote add origin https://" + owner.Spec.SourceRepo,
							"git fetch --depth=1 origin " + owner.Spec.SourceRef,
							"git checkout FETCH_HEAD",
						}, "\n"),
					},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "k6-script",
						MountPath: "/scripts",
					}},
				})
		}

		containers := obj.Spec.Containers
		probe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/v1/status",
					Port:   intstr.IntOrString{IntVal: 6565},
					Scheme: "HTTP",
				},
			},
		}
		if corev1util.GetContainerByName(containers, "k6") == nil {
			obj.Spec.Containers = corev1util.UpsertContainer(obj.Spec.Containers,
				corev1.Container{
					Name: "k6",
					// Image:           "loadimpact/k6",
					Image: "europe-west4-docker.pkg.dev/calins-playground/k6-testing/k6-influxdb:latest",

					ImagePullPolicy: corev1.PullIfNotPresent,
					WorkingDir:      "/scripts",
					Command: []string{"/bin/sh", "-c",
						strings.Join([]string{
							script,
							"EXIT_CODE=$?",
							// 99 is the k6 exit code for ThresholdsHaveFailed.
							// This is not an error from the operator's perspective.
							"if [ $EXIT_CODE -ne 0 ] && [ $EXIT_CODE -ne 99 ]; then exit $EXIT_CODE; fi",
						}, "\n"),
					},
					Env: []corev1.EnvVar{
						{
							Name:  "TARGET",
							Value: owner.Spec.Target,
						},
					},
					Ports: []corev1.ContainerPort{{
						Name:          "http-api",
						ContainerPort: 6565,
					}},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "k6-script",
						MountPath: "/scripts",
					}},
					LivenessProbe:  probe,
					ReadinessProbe: probe,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    job.TestRun.ResourceCPU,
							corev1.ResourceMemory: job.TestRun.ResourceMemory,
						},
					},
				},
			)
		}

		return nil
	})

	return err
}

func isPodCompleted(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func isPodSucceeded(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded
}

func isPodFailed(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed
}

func isPodStableReady(pod *corev1.Pod) bool {
	now := time.Now()
	stabilityPeriod := 2 * time.Minute
	stabilityPeriod = 1 * time.Second

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.LastTransitionTime.Add(stabilityPeriod).Before(now) {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *TestRunReconciler) createTestRun(ctx context.Context, job *loadtestingapi.Job) (*loadtestingv1alpha1.TestRun, error) {
	obj, ok := job.ToK8SResource().(*loadtestingv1alpha1.TestRun)
	if !ok {
		return nil, fmt.Errorf("failed to convert job to TestRun")
	}

	err := r.Create(ctx, obj)
	return obj, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clientset, err := clientset.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.clientset = clientset

	worker, err := loadtesting.NewWorker(r.APIClient, &loadtestingapi.Job{}, func(obj loadtestingruntime.Object) bool {
		job, ok := obj.(*loadtestingapi.Job)
		if !ok {
			return false
		}

		if job.Status == loadtestingapi.STATUS_COMPLETED || job.Status == loadtestingapi.STATUS_FAILED {
			return false
		}

		return true
	})
	if err != nil {
		return err
	}

	if err = mgr.Add(worker); err != nil {
		return err
	}

	r.igniters = make(Igniters)

	return ctrl.NewControllerManagedBy(mgr).
		For(&loadtestingv1alpha1.TestRun{}).
		Owns(&corev1.Pod{}).
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
