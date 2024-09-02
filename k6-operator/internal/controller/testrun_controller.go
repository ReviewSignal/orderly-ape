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
	policyv1 "k8s.io/api/policy/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting"
	loadtestingapi "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/api"
	loadtestingruntime "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

const telegrafConfigVersion = 1

var (
	zero32   int32 = 0
	falsePtr *bool = func(b bool) *bool { return &b }(false)
	truePtr  *bool = func(b bool) *bool { return &b }(true)
	userID   int64 = 65534
	groupID  int64 = 65534

	telegrafIntervalSeconds      = 5
	telegrafFlushIntervalSeconds = 10
	telegrafFlushJitterSeconds   = 5

	K6Image       string
	TelegrafImage string
)

func init() {
	if K6Image == "" {
		K6Image = "ghcr.io/reviewsignal/orderly-ape/k6:latest"
	}

	if TelegrafImage == "" {
		TelegrafImage = "telegraf:1.31.1-alpine"
	}
}

// TestRunReconciler reconciles a TestRun object
type TestRunReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	APIClient loadtesting.Client
	Location  string
	clientset clientset.Interface
	igniters  Igniters
}

//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=create
//+kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs/finalizers,verbs=update

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
	if loadtesting.IgnoreNotFound(err) != nil {
		l.Error(err, "Failed retrieving Job from API", "job", job)
		return ctrl.Result{}, err
	}

	if loadtesting.IsNotFound(err) {
		bgDelete := metav1.DeletePropagationBackground
		err = r.Delete(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
		}, &client.DeleteOptions{
			PropagationPolicy: &bgDelete,
		})
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	l = l.WithValues("status", job.Status)

	// If the job is completed or failed, we don't need to do anything
	if job.Status == loadtestingapi.STATUS_COMPLETED || job.Status == loadtestingapi.STATUS_FAILED {
		r.removeIgniter(job)
		return ctrl.Result{}, nil
	}

	obj := &batchv1.Job{}
	err = r.Get(ctx, req.NamespacedName, obj)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if obj.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// If the job is canceled, we need to suspend the job
	if job.Status == loadtestingapi.STATUS_CANCELED {
		// If the Job exists in kubernetes, we need to suspend it
		if err == nil && (obj.Spec.Suspend == nil || !*obj.Spec.Suspend) {
			l.Info("Suspending canceled TestRun")
			obj.Spec.Suspend = truePtr
			err = r.Update(ctx, obj)
		}
		r.removeIgniter(job)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// From here on, we are handling jobs that are not completed, failed or canceled
	l.Info("Reconciling TestRun")

	if apierrors.IsNotFound(err) && job.Status != loadtestingapi.STATUS_PENDING {
		job.Status = loadtestingapi.STATUS_FAILED
		job.StatusDescription = fmt.Sprintf("Test was `%s` but no Kubernetes Job found", job.Status)
		err = r.APIClient.Update(ctx, job)
		if err != nil {
			l.Error(err, "Failed updating job status", "job", job)
		}
		return ctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) && job.Status == loadtestingapi.STATUS_PENDING {
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

		_, err = r.syncTelegrafConfig(ctx, job, obj)
		if err != nil {
			return ctrl.Result{}, err
		}

		_, err = r.syncPodDisruptionBudget(ctx, job, obj)
		if err != nil {
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

	if job.Status == loadtestingapi.STATUS_READY {
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

func (r *TestRunReconciler) syncPodDisruptionBudget(ctx context.Context, job *loadtestingapi.Job, parent *batchv1.Job) (*policyv1.PodDisruptionBudget, error) {
	obj := &policyv1.PodDisruptionBudget{
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

		err := controllerutil.SetOwnerReference(parent, obj, r.Scheme)
		if err != nil {
			return err
		}

		obj.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"batch.kubernetes.io/job-name": job.GetName(),
			},
		}

		count := len(job.AssignedSegments)
		replicas := intstr.FromInt(count)
		obj.Spec.MinAvailable = &replicas

		return nil
	})

	return obj, err
}

func (r *TestRunReconciler) syncTelegrafConfig(ctx context.Context, job *loadtestingapi.Job, parent *batchv1.Job) (*corev1.Secret, error) {
	obj := &corev1.Secret{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", job.GetName(), telegrafConfigVersion),
			Namespace: job.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/name":       "k6",
				"app.kubernetes.io/instance":   job.GetName(),
				"app.kubernetes.io/managed-by": "orderly-ape",
			},
		},
		StringData: map[string]string{
			"telegraf.conf": fmt.Sprintf(`
[agent]
interval = "%ds"
flush_interval = "%ds"
flush_jitter = "%ds"

# Statsd Server
[[inputs.statsd]]
  ## Protocol, must be "tcp", "udp4", "udp6" or "udp" (default=udp)
  protocol = "udp"

  ## Address and port to host UDP listener on
  service_address = ":8125"

  ## Percentiles to calculate for timing & histogram stats.
  percentiles = [90.0, 95.0, 99.0, 99.9, 99.95]

  ## Parses extensions to statsd in the datadog statsd format
  ## currently supports metrics and datadog tags.
  ## http://docs.datadoghq.com/guides/dogstatsd/
  datadog_extensions = true

[[outputs.influxdb_v2]]
  ## The URLs of the InfluxDB cluster nodes.
  ##
  ## Multiple URLs can be specified for a single cluster, only ONE of the
  ## urls will be written to each interval.
  ##   ex: urls = ["https://us-west-2-1.aws.cloud2.influxdata.com"]
  urls = ["%s"]

  ## Token for authentication.
  token = "%s"

  ## Organization is the name of the organization you wish to write to.
  organization = "%s"

  ## Destination bucket to write into.
  bucket = "%s"

  ## Use TLS but skip chain & host verification
  insecure_skip_verify = %t
            `,
				telegrafIntervalSeconds,
				telegrafFlushIntervalSeconds,
				telegrafFlushJitterSeconds,
				job.OutputConfig.InfluxURL,
				job.OutputConfig.InfluxToken,
				job.OutputConfig.InfluxOrganization,
				job.OutputConfig.InfluxBucket,
				job.OutputConfig.TLSSkipVerify),
		},
	}

	err := controllerutil.SetOwnerReference(parent, obj, r.Scheme)
	if err != nil {
		return nil, err
	}

	err = r.Create(ctx, obj)
	if client.IgnoreAlreadyExists(err) != nil {
		return nil, err
	}

	return obj, nil
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
		ttlSecondsAferFinished := int32(3600) // keep the job for 1 hour after it finishes
		obj.Spec.TTLSecondsAfterFinished = &ttlSecondsAferFinished
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

		gracePeriod := max(10, int64(telegrafFlushIntervalSeconds*2+telegrafFlushJitterSeconds))

		pod.Spec.RestartPolicy = corev1.RestartPolicyNever
		pod.Spec.TerminationGracePeriodSeconds = &gracePeriod
		pod.Spec.ShareProcessNamespace = truePtr
		pod.Spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup:    &groupID,
			RunAsUser:  &userID,
			RunAsGroup: &groupID,
		}

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

		command := []string{"k6", "run", "--paused", "--address", "0.0.0.0:6565"}
		if verbose := job.TestRun.EnvVars["K6_VERBOSE"]; verbose == "true" {
			command = append(command, "--verbose")
		}

		tags := job.TestRun.Labels
		tags["testid"] = job.GetName()
		tags["location"] = r.Location

		segmentsEnv := make([]corev1.EnvVar, 0)
		command = append(command, "--execution-segment-sequence", strings.Join(job.TestRun.Segments, ","))
		command = append(command, "--execution-segment", "__ASSIGNED_SEGMENT__")
		tags["segment_number"] = "__ASSIGNED_SEGMENT_ID__"
		for i, segment := range job.AssignedSegments {
			segmentsEnv = append(segmentsEnv,
				corev1.EnvVar{
					Name:  fmt.Sprintf("SEGMENT_%d", i),
					Value: segment.Segment,
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("SEGMENT_ID_%d", i),
					Value: segment.ID,
				},
			)
		}

		for key, value := range tags {
			command = append(command, "--tag", fmt.Sprintf("%s=%s", key, value))
		}

		command = append(command, job.TestRun.SourceScript)

		script := shellescape.QuoteCommand(command)
		script = strings.Replace(script, "__ASSIGNED_SEGMENT__", `"${SEGMENT_$(JOB_COMPLETION_INDEX)}"`, -1)
		script = strings.Replace(script, "__ASSIGNED_SEGMENT_ID__", `"${SEGMENT_ID_$(JOB_COMPLETION_INDEX)}"`, -1)

		if corev1util.GetVolumeByName(pod.Spec.Volumes, "k6-script") == nil {
			pod.Spec.Volumes = corev1util.UpsertVolume(pod.Spec.Volumes,
				corev1.Volume{
					Name: "k6-script",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			)
			pod.Spec.Volumes = corev1util.UpsertVolume(pod.Spec.Volumes,
				corev1.Volume{
					Name: "telegraf-config",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: fmt.Sprintf("%s-%d", job.GetName(), telegrafConfigVersion),
						},
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
							"mkdir -p /tmp/nobody",
							"export HOME=/tmp/nobody",
							"set -eo pipefail",
							"set -x",
							"git config --global --add safe.directory '/scripts'",
							"git init -q",
							"git remote add origin https://" + job.TestRun.SourceRepo,
							"git fetch -q --depth=1 origin " + job.TestRun.SourceRef,
							"git checkout -q FETCH_HEAD",
						}, "\n"),
					},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "k6-script",
						MountPath: "/scripts",
					}},
				})
		}

		probe := &corev1.Probe{
			InitialDelaySeconds: 30,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/v1/status",
					Port:   intstr.IntOrString{IntVal: 6565},
					Scheme: "HTTP",
				},
			},
		}

		env := []corev1.EnvVar{}
		for name, value := range job.TestRun.EnvVars {
			env = corev1util.UpsertEnvVars(env, corev1.EnvVar{
				Name:  name,
				Value: value,
			})
		}
		env = corev1util.UpsertEnvVars(env,
			corev1.EnvVar{
				Name:  "TARGET",
				Value: job.TestRun.Target,
			},
			corev1.EnvVar{
				Name:  "K6_OUT",
				Value: "output-statsd",
			},
			corev1.EnvVar{
				Name:  "K6_STATSD_ENABLE_TAGS",
				Value: "true",
			},
		)

		if segmentsEnv != nil {
			env = corev1util.UpsertEnvVars(env, segmentsEnv...)
		}

		pullPolicy := corev1.PullIfNotPresent
		if strings.HasSuffix(K6Image, ":latest") {
			pullPolicy = corev1.PullAlways
		}

		pod.Spec.Containers = corev1util.UpsertContainer(pod.Spec.Containers,
			corev1.Container{
				Name:            "k6",
				Image:           K6Image,
				ImagePullPolicy: pullPolicy,
				WorkingDir:      "/scripts",
				Command: []string{"/bin/sh", "-c",
					fmt.Sprintf(`
                        PID=""
                        terminate() {
                            echo "Received TERM signal. Killing k6" >&2
                            if [ -n "$PID" ] ; then kill -TERM $PID || true ; fi
                        }
                        trap 'terminate' TERM
                        %s & PID=$!
                        wait $PID
                        EXIT_CODE=$?
                        echo "k6 finished with code $EXIT_CODE" >&2
                        echo "Allow telegraf to flush it's metrics" >&2
                        sleep %d
                        echo "Killing telegraf" >&2
                        killall telegraf || true
                        # 99 is the k6 exit code for ThresholdsHaveFailed.
                        # This is not an error from the operator's perspective.
                        if [ $EXIT_CODE -ne 0 ] && [ $EXIT_CODE -ne 99 ] ; then exit $EXIT_CODE ; fi
                        exit 0
                    `, script, gracePeriod),
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

		pod.Spec.Containers = corev1util.UpsertContainer(pod.Spec.Containers,
			corev1.Container{
				Name:            "telegraf",
				Image:           TelegrafImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "telegraf-config",
					MountPath: "/etc/telegraf",
				}},
				Lifecycle: &corev1.Lifecycle{
					PreStop: &corev1.LifecycleHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"/bin/sh", "-c",
								fmt.Sprintf(`
									echo "Allow telegraf to flush it's metrics" >&2
									sleep %d
								`, telegrafFlushIntervalSeconds),
							},
						},
					},
				},
			},
		)

		obj.Spec.Template = *pod

		return nil
	})

	return obj, err
}

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

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
	stabilityPeriod := 5 * time.Second

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
