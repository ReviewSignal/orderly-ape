//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2024 ReviewSignal

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alessio/shellescape"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	corev1util "kmodules.xyz/client-go/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting"
	loadtestingapi "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/api"
	loadtestingruntime "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

var (
	zero64   int64 = 0
	zero32   int32 = 0
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

//+kubebuilder:rbac:groups=batch,resources=Job,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=Job/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=Job/finalizers,verbs=update

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

	obj := &batchv1.Job{}
	err = r.Get(ctx, req.NamespacedName, obj)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if obj.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) {
		obj, err = r.syncJob(ctx, job)
		if err != nil {
			job.Status = loadtestingapi.STATUS_FAILED
			job.StatusDescription = fmt.Sprintf("Worker pods have failed running k6 tests: %s", err)
			err = r.APIClient.Update(ctx, job)
			if err != nil {
				l.Error(err, "Failed updating job status", "job", job)
			}
			return ctrl.Result{}, err
		}
	}

	if cond := getJobCondition(obj, batchv1.JobFailed); cond != nil {
		job.Status = loadtestingapi.STATUS_FAILED
		job.StatusDescription = fmt.Sprintf("Worker pods have failed running k6 tests: %s", cond.Message)
		err = r.APIClient.Update(ctx, job)
		if err != nil {
			l.Error(err, "Failed updating job status", "job", job)
		}
		return ctrl.Result{}, nil
	}

	if job.Status == loadtestingapi.STATUS_PENDING {
		job.Status = loadtestingapi.STATUS_QUEUED
		job.StatusDescription = "Test run is queued for execution"
		err = r.APIClient.Update(ctx, job)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if job.Status == loadtestingapi.STATUS_QUEUED {
		if obj.Status.Ready != nil && int32(len(job.AssignedSegments)) == *obj.Status.Ready {
			pods, err := r.getPods(ctx, job)
			if err != nil {
				return ctrl.Result{}, err
			}
			ready := true
			for _, pod := range pods {
				if !isPodStableReady(&pod) {
					ready = false
				}
			}
			if ready {
				job.Status = loadtestingapi.STATUS_READY
				job.StatusDescription = "Worker pods are ready and waiting to start testing"
				err = r.APIClient.Update(ctx, job)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	if job.Status == loadtestingapi.STATUS_READY && job.TestRun.Ready {
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
			job.StatusDescription = fmt.Sprintf("Worker pods have failed running k6 tests: %s", igniter.Error)
			err = r.APIClient.Update(ctx, job)
			if err != nil {
				l.Error(err, "Failed updating job status", "job", job)
			}
		}
	}

	if job.Status == loadtestingapi.STATUS_RUNNING {
		if obj.Status.Active == 0 {
			if int(obj.Status.Succeeded) == len(job.AssignedSegments) {
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

func (r *TestRunReconciler) syncJob(ctx context.Context, job *loadtestingapi.Job) (*batchv1.Job, error) {
	obj := &batchv1.Job{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      job.GetName(),
			Namespace: job.GetNamespace(),
		},
	}
	_, err := ctrl.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if len(obj.Labels) == 0 {
			obj.Labels = make(map[string]string)
		}
		obj.Labels["app.kubernetes.io/name"] = "k6"
		obj.Labels["app.kubernetes.io/instance"] = job.GetName()
		obj.Labels["app.kubernetes.io/managed-by"] = "orderly-ape"

		count := int32(len(job.AssignedSegments))
		obj.Spec.Parallelism = &count
		obj.Spec.Completions = &count
		obj.Spec.BackoffLimit = &zero32
		indexed := batchv1.IndexedCompletion
		obj.Spec.CompletionMode = &indexed
		if job.TestRun.JobDeadline != nil {
			activeDeadlineSeconds := int64(job.TestRun.JobDeadline.Seconds())
			obj.Spec.ActiveDeadlineSeconds = &activeDeadlineSeconds
		}

		pod := &corev1.PodTemplateSpec{}

		pod.Spec.RestartPolicy = corev1.RestartPolicyNever
		pod.Spec.TerminationGracePeriodSeconds = &zero64

		pod.Spec.NodeSelector = job.TestRun.NodeSelector

		if job.TestRun.DedicatedNodes && pod.Spec.Affinity == nil {
			pod.Spec.Affinity = &corev1.Affinity{
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
			"--tag", fmt.Sprintf("testid=%s", job.GetName()),
			"--tag", fmt.Sprintf("location=%s", r.Location),
		}

		var segmentsEnv []corev1.EnvVar
		if len(job.AssignedSegments) > 1 {
			command = append(command, "--execution-segment-sequence", strings.Join(job.TestRun.Segments, ","))
			command = append(command, "--execution-segment", "__ASSIGNED_SEGMENT__")
			segmentsEnv = make([]corev1.EnvVar, len(job.AssignedSegments))
			for i, segment := range job.AssignedSegments {
				segmentsEnv[i].Name = fmt.Sprintf("SEGMENT_%d", i)
				segmentsEnv[i].Value = segment
			}
		}

		command = append(command, job.TestRun.SourceScript)

		script := shellescape.QuoteCommand(command)
		script = strings.Replace(script, "__ASSIGNED_SEGMENT__", `"${SEGMENT_$(JOB_COMPLETION_INDEX)}"`, -1)

		if corev1util.GetVolumeByName(pod.Spec.Volumes, "k6-script") == nil {
			pod.Spec.Volumes = corev1util.UpsertVolume(pod.Spec.Volumes,
				corev1.Volume{
					Name: "k6-script",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			)
		}

		if corev1util.GetContainerByName(pod.Spec.InitContainers, "git") == nil {
			pod.Spec.InitContainers = corev1util.UpsertContainer(pod.Spec.InitContainers,
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
							"git remote add origin https://" + job.TestRun.SourceRepo,
							"git fetch --depth=1 origin " + job.TestRun.SourceRef,
							"git checkout FETCH_HEAD",
						}, "\n"),
					},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "k6-script",
						MountPath: "/scripts",
					}},
				})
		}

		probe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/v1/status",
					Port:   intstr.IntOrString{IntVal: 6565},
					Scheme: "HTTP",
				},
			},
		}

		env := []corev1.EnvVar{
			{
				Name:  "TARGET",
				Value: job.TestRun.Target,
			},
		}
		if segmentsEnv != nil {
			env = corev1util.UpsertEnvVars(env, segmentsEnv...)
		}

		pod.Spec.Containers = corev1util.UpsertContainer(pod.Spec.Containers,
			corev1.Container{
				Name: "k6",
				// Image:           "loadimpact/k6",
				Image: "europe-west4-docker.pkg.dev/calins-playground/k6-testing/k6-influxdb:latest",

				ImagePullPolicy: corev1.PullIfNotPresent,
				WorkingDir:      "/scripts",
				Command: []string{"/bin/sh", "-c",
					strings.Join([]string{
						script,
						`EXIT_CODE=$?`,
						// 99 is the k6 exit code for ThresholdsHaveFailed.
						// This is not an error from the operator's perspective.
						`if [ $EXIT_CODE -ne 0 ] && [ $EXIT_CODE -ne 99 ]; then exit $EXIT_CODE; fi`,
					}, "\n"),
				},
				Env: env,
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

		obj.Spec.Template = *pod

		return nil
	})

	return obj, err
}

func (r *TestRunReconciler) getPods(ctx context.Context, job *loadtestingapi.Job) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}
	err := r.Client.List(ctx, pods, client.InNamespace(job.GetNamespace()), client.MatchingLabels{
		"batch.kubernetes.io/job-name": job.GetName(),
	})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func getJobCondition(job *batchv1.Job, conditionType batchv1.JobConditionType) *batchv1.JobCondition {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
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

	managedByOrderlyApe, err := predicate.LabelSelectorPredicate(
		metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app.kubernetes.io/managed-by": "orderly-ape",
			},
		},
	)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{},
			builder.WithPredicates(
				managedByOrderlyApe,
			),
		).
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
