// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"strings"
	"text/template"
	"time"

	"al.essio.dev/pkg/shellescape"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1util "kmodules.xyz/client-go/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

var telegrafConfigTemplate = template.Must(template.New("telegraf.conf").Parse(`[agent]
interval = "{{ .Interval }}"
flush_interval = "{{ .FlushInterval }}"
flush_jitter = "{{ .FlushJitter }}"

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

  ## Convert all numeric counters to float
  ## Enabling this would ensure that both counters and guages are both emitted
  ## as floats.
  float_counters = true

  ## Emit timings metric_<name>_count field as float, the same as all other
  ## histogram fields
  float_timings = true

  ## Emit sets as float
  float_sets = true

# Kubernetes resource monitoring for the k6 pod
[[inputs.kubernetes]]
  ## URL for the kubelet, if empty read-only port 10255 is used
  url = "https://${HOST_IP}:10250"
  tls_ca = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

  [inputs.kubernetes.tags]
    testid = "{{ .TestID }}"
    location = "{{ .Worker }}"
    job_index = "${JOB_COMPLETION_INDEX}"

  [inputs.kubernetes.tagpass]
    pod_name = ["{{ .TestID }}-*"]

{{ if .VictoriaLogs }}
# Read logs from container log files
[[inputs.tail]]
  files = ["/var/log/containers/*.log"]
  initial_read_offset = "saved-or-beginning"
  watch_method = "inotify"
  name_override = "log"
  data_format = "value"
  data_type = "string"
  character_encoding = "utf-8"
  metricpass = 'has(fields.value) && fields.value != ""'

  [inputs.tail.tags]
    testid = "{{ .VictoriaLogs.TestID }}"
    location = "{{ .VictoriaLogs.Worker }}"
    job_index = "${JOB_COMPLETION_INDEX}"
[[processors.starlark]]
  namepass = ["log"]
  source = '''
state = {}

def apply(metric):
    path = metric.tags.get("path", "unknown")
    seq = state.get(path, 0) + 1
    state[path] = seq
    metric.fields["seq"] = seq
    if "value" in metric.fields:
        metric.fields["message"] = metric.fields.pop("value")
    return metric
  '''
{{ end }}
[[outputs.influxdb_v2]]
  ## The URLs of the InfluxDB cluster nodes.
  ##
  ## Multiple URLs can be specified for a single cluster, only ONE of the
  ## urls will be written to each interval.
  ##   ex: urls = ["https://us-west-2-1.aws.cloud2.influxdata.com"]
  urls = ["{{ .InfluxDB.Host }}"]

  ## Token for authentication.
  token = "{{ .InfluxDB.Token }}"

  ## Organization is the name of the organization you wish to write to.
  organization = "{{ .InfluxDB.Org }}"

  ## Destination bucket to write into.
  bucket = "{{ .InfluxDB.Bucket }}"

  ## Use TLS but skip chain & host verification
  insecure_skip_verify = false

  ## Do not send logs
  namedrop = ["log"]

{{ if .VictoriaLogs }}
# Send logs to VictoriaLogs
[[outputs.http]]
  url = "{{ .VictoriaLogs.InsertURL }}/jsonline?_msg_field=fields.message&_time_field=timestamp&_stream_fields=tags.testid,tags.location,tags.job_index,tags.path"
  method = "POST"
  data_format = "json"
  use_batch_format = false
  ## Only send log entries
  namepass = ["log"]

  [outputs.http.headers]
    Content-Type = "application/json"
{{ if .VictoriaLogs.Token }}
    Authorization = "Bearer {{ .VictoriaLogs.Token }}"
{{ end }}
{{ end }}
`))

func (r *TestRunWorkerReconciler) syncJob(ctx context.Context, worker *apev1.Worker, testrun *apev1.TestRun) (*batchv1.Job, error) {
	var err error

	cl, err := r.clientFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var workerSpec *apev1.TestRunWorkerSpec
	for i, w := range testrun.Spec.Workers {
		if w.Name == worker.Name {
			workerSpec = &testrun.Spec.Workers[i]
		}
	}
	if workerSpec == nil {
		return nil, fmt.Errorf("invalid TestRun config: Worker '%s' not in .spec", worker.Name)
	}

	var workerStatus *apev1.TestRunWorkerStatus
	for i, w := range testrun.Status.WorkerStatuses {
		if w.Name == worker.Name {
			workerStatus = &testrun.Status.WorkerStatuses[i]
		}
	}
	if workerStatus == nil {
		return nil, fmt.Errorf("invalid TestRun status: Worker '%s' not in .status", worker.Name)
	}

	obj := &batchv1.Job{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      testrun.Name,
			Namespace: worker.Spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "k6",
				"app.kubernetes.io/instance":   testrun.Name,
				"app.kubernetes.io/managed-by": "orderly-ape",
			},
		},
	}

	count := workerSpec.NumWorkers
	if count <= 0 {
		count = 1
	}

	ttlSecondsAfterFinished := int32(3600) // keep the job for 1 hour after it finishes
	obj.Spec.TTLSecondsAfterFinished = &ttlSecondsAfterFinished
	obj.Spec.Parallelism = &count
	obj.Spec.Completions = &count
	obj.Spec.BackoffLimit = &zero32
	indexed := batchv1.IndexedCompletion
	obj.Spec.CompletionMode = &indexed
	if testrun.Status.ScenarioSpec.MaxDuration != nil {
		activeDeadlineSeconds := int64(testrun.Status.ScenarioSpec.MaxDuration.Seconds())
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

	pod.Spec.NodeSelector = worker.Spec.NodeSelector
	pod.Spec.Tolerations = worker.Spec.Tolerations

	if testrun.Spec.PlacementStrategy == apev1.PlacementSpreadOut && pod.Spec.Affinity == nil {
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

	tags := make(map[string]string)
	maps.Copy(tags, testrun.Spec.Labels)
	tags["testid"] = testrun.Name
	tags["location"] = worker.Name

	segmentsEnv := make([]corev1.EnvVar, 0)
	command = append(command, "--execution-segment-sequence", strings.Join(testrun.SegmentSequence(), ","))
	command = append(command, "--execution-segment", "__ASSIGNED_SEGMENT__")
	tags["segment_number"] = "__ASSIGNED_SEGMENT_ID__"
	for i, segment := range workerStatus.AssignedSegments {
		idx := 0
		segments := testrun.Segments()
		for i := range segments {
			if segments[i] == segment {
				idx = i + 1
				break
			}
		}
		if idx == 0 {
			return nil, fmt.Errorf("invalid TestRun status: segment '%s' not in .status.segments", segment)
		}
		segmentsEnv = append(segmentsEnv,
			corev1.EnvVar{
				Name:  fmt.Sprintf("SEGMENT_%d", i),
				Value: segment,
			},
			corev1.EnvVar{
				Name:  fmt.Sprintf("SEGMENT_ID_%d", i),
				Value: fmt.Sprintf("%d", idx),
			},
		)
	}

	for key, value := range tags {
		command = append(command, "--tag", fmt.Sprintf("%s=%s", key, value))
	}

	command = append(command, testrun.Status.ScenarioSpec.GitRepo.Path)

	script := shellescape.QuoteCommand(command)
	script = strings.ReplaceAll(script, "__ASSIGNED_SEGMENT__", `"${SEGMENT_$(JOB_COMPLETION_INDEX)}"`)
	script = strings.ReplaceAll(script, "__ASSIGNED_SEGMENT_ID__", `"${SEGMENT_ID_$(JOB_COMPLETION_INDEX)}"`)

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
						SecretName: fmt.Sprintf("%s-%d", testrun.Name, telegrafConfigVersion),
					},
				},
			},
		)
		pod.Spec.Volumes = corev1util.UpsertVolume(pod.Spec.Volumes,
			corev1.Volume{
				Name: "container-logs",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)
	}

	if corev1util.GetContainerByName(pod.Spec.InitContainers, "git") == nil {
		repo := testrun.Status.ScenarioSpec.GitRepo.Repository
		pod.Spec.InitContainers = corev1util.UpsertContainer(pod.Spec.InitContainers,
			corev1.Container{
				Name:            "git",
				Image:           "alpine/git",
				ImagePullPolicy: corev1.PullIfNotPresent,
				WorkingDir:      "/scripts",
				Command: []string{"/bin/sh", "-c",
					strings.Join([]string{
						"mkdir -p /tmp/nobody /var/log/containers",
						"export HOME=/tmp/nobody",
						"set -eo pipefail",
						"exec > >(tee /var/log/containers/git.log) 2>&1",
						"set -x",
						"git config --global --add safe.directory '/scripts'",
						"git init -q",
						"git remote add origin " + repo,
						"git fetch -q --depth=1 origin " + testrun.Status.ScenarioSpec.GitRepo.Revision,
						"git checkout -q FETCH_HEAD",
					}, "\n"),
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "k6-script",
						MountPath: "/scripts",
					},
					{
						Name:      "container-logs",
						MountPath: "/var/log/containers",
					},
				},
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
	for _, v := range testrun.Spec.EnvVars {
		env = corev1util.UpsertEnvVars(env, corev1.EnvVar{
			Name:  v.Name,
			Value: v.Value,
		})
	}
	env = corev1util.UpsertEnvVars(env,
		corev1.EnvVar{
			Name:  "TARGET",
			Value: testrun.Spec.Target,
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

	resources := corev1.ResourceRequirements{}
	if testrun.Status.ScenarioSpec.Resources != nil {
		resources = *testrun.Status.ScenarioSpec.Resources
	}
	pod.Spec.Containers = corev1util.UpsertContainer(pod.Spec.Containers,
		corev1.Container{
			Name:            "k6",
			Image:           K6Image,
			ImagePullPolicy: pullPolicy,
			WorkingDir:      "/scripts",
			Command: []string{"/bin/sh", "-c",
				fmt.Sprintf(`
                        mkdir -p /var/log/containers
                        exec > >(tee /var/log/containers/k6.log) 2>&1
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
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "k6-script",
					MountPath: "/scripts",
				},
				{
					Name:      "container-logs",
					MountPath: "/var/log/containers",
				},
			},
			LivenessProbe:  probe,
			ReadinessProbe: probe,
			Resources:      resources,
		},
	)

	pod.Spec.InitContainers = corev1util.UpsertContainer(pod.Spec.InitContainers,
		corev1.Container{
			Name:            "telegraf",
			Image:           TelegrafImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			RestartPolicy:   func() *corev1.ContainerRestartPolicy { p := corev1.ContainerRestartPolicyAlways; return &p }(),
			Env: []corev1.EnvVar{
				{
					Name: "POD_NAME",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.name",
						},
					},
				},
				{
					Name: "HOST_IP",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "status.hostIP",
						},
					},
				},
				{
					Name: "JOB_COMPLETION_INDEX",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.annotations['batch.kubernetes.io/job-completion-index']",
						},
					},
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "telegraf-config",
					MountPath: "/etc/telegraf",
				},
				{
					Name:      "container-logs",
					MountPath: "/var/log/containers",
				},
			},
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh", "-c",
							fmt.Sprintf(`
									echo "Allow telegraf to flush it's metrics" >&2
									sleep %d
								`, 2*telegrafFlushIntervalSeconds),
						},
					},
				},
			},
		},
	)

	obj.Spec.Template = *pod

	err = cl.Create(ctx, obj)
	if client.IgnoreAlreadyExists(err) != nil {
		return nil, err
	}

	return obj, err
}

func (r *TestRunWorkerReconciler) syncPodDisruptionBudget(ctx context.Context, worker *apev1.Worker, testrun *apev1.TestRun, job *batchv1.Job) (*policyv1.PodDisruptionBudget, error) {
	var err error

	cl, err := r.clientFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var workerSpec *apev1.TestRunWorkerSpec
	for i, w := range testrun.Spec.Workers {
		if w.Name == worker.Name {
			workerSpec = &testrun.Spec.Workers[i]
		}
	}
	if workerSpec == nil {
		return nil, fmt.Errorf("invalid TestRun config: worker '%s' not in .spec", worker.Name)
	}

	var workerStatus *apev1.TestRunWorkerStatus
	for i, w := range testrun.Status.WorkerStatuses {
		if w.Name == worker.Name {
			workerStatus = &testrun.Status.WorkerStatuses[i]
		}
	}
	if workerStatus == nil {
		return nil, fmt.Errorf("invalid TestRun status: worker '%s' not in .status", worker.Name)
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      job.GetName(),
			Namespace: job.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/name":       "k6",
				"app.kubernetes.io/instance":   testrun.Name,
				"app.kubernetes.io/managed-by": "orderly-ape",
			},
		},
	}
	err = controllerutil.SetOwnerReference(job, pdb, r.Scheme)
	if err != nil {
		return nil, err
	}

	pdb.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"batch.kubernetes.io/job-name": testrun.Name,
		},
	}

	count := len(workerStatus.AssignedSegments)
	replicas := intstr.FromInt(count)
	pdb.Spec.MinAvailable = &replicas

	err = cl.Create(ctx, pdb)
	if client.IgnoreAlreadyExists(err) != nil {
		return nil, err
	}

	return pdb, err
}

func (r *TestRunWorkerReconciler) syncTelegrafConfig(ctx context.Context, worker *apev1.Worker, testrun *apev1.TestRun, job *batchv1.Job, creds *corev1.Secret) (*corev1.Secret, error) {
	var err error

	cl, err := r.clientFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Prepare VictoriaLogs configuration
	var victoriaLogsConfig map[string]any
	if testrun.Status.VictoriaLogs != nil && r.VictoriaLogsInsertURL != "" {
		victoriaLogsConfig = map[string]any{
			"InsertURL": r.VictoriaLogsInsertURL,
			"TestID":    testrun.Name,
			"Worker":    worker.Name,
			"ProjectID": fmt.Sprintf("%d", testrun.Status.VictoriaLogs.ProjectID),
		}

		// Generate JWT token if proxied insert is used
		if r.ProxiedVictoriaLogsInsert && r.JWTManager != nil {
			// Calculate token expiry: test timeout + 10 minutes margin
			tokenExpiry := 40 * time.Minute // default
			if testrun.Status.ScenarioSpec != nil && testrun.Status.ScenarioSpec.MaxDuration != nil {
				tokenExpiry = testrun.Status.ScenarioSpec.MaxDuration.Duration + 10*time.Minute
			}

			// Generate JWT token with testrun and project info
			token, err := r.JWTManager.GenerateToken(
				testrun.Name,
				tokenExpiry,
				"testrun", testrun.Name,
				"projectID", testrun.Status.VictoriaLogs.ProjectID,
				"accountID", testrun.Status.VictoriaLogs.AccountID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to generate JWT token for VictoriaLogs: %w", err)
			}
			victoriaLogsConfig["Token"] = token
		}
	}

	var buf bytes.Buffer
	err = telegrafConfigTemplate.Execute(&buf, map[string]any{
		"Interval":      time.Duration(telegrafIntervalSeconds) * time.Second,
		"FlushInterval": time.Duration(telegrafFlushIntervalSeconds) * time.Second,
		"FlushJitter":   time.Duration(telegrafFlushJitterSeconds) * time.Second,
		"TestID":        testrun.Name,
		"Worker":        worker.Name,
		"InfluxDB": map[string]string{
			"Host":   string(creds.Data["INFLUXDB_HOST"]),
			"Token":  string(creds.Data["INFLUXDB_TOKEN"]),
			"Org":    string(creds.Data["INFLUXDB_ORG"]),
			"Bucket": string(creds.Data["INFLUXDB_BUCKET"]),
		},
		"VictoriaLogs": victoriaLogsConfig,
	})
	if err != nil {
		return nil, err
	}

	obj := &corev1.Secret{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", testrun.Name, telegrafConfigVersion),
			Namespace: job.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/name":       "k6",
				"app.kubernetes.io/instance":   testrun.Name,
				"app.kubernetes.io/managed-by": "orderly-ape",
			},
		},
		StringData: map[string]string{
			"telegraf.conf": buf.String(),
		},
	}

	err = controllerutil.SetOwnerReference(job, obj, r.Scheme)
	if err != nil {
		return nil, err
	}

	err = cl.Create(ctx, obj)
	if client.IgnoreAlreadyExists(err) != nil {
		return nil, err
	}

	return obj, nil
}
