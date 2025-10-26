// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/ReviewSignal/orderly-ape/internal/grafana"
	"github.com/ReviewSignal/orderly-ape/internal/info"
	safename "github.com/ReviewSignal/orderly-ape/internal/util/name"
	"github.com/ReviewSignal/orderly-ape/internal/util/sourcedvalue"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

const (
	testRunFinalizerInfluxDBCredentials = "ape.reviewsignal.com/influxdb-credentials"
)

const telegrafConfigVersion = 2

const podReadyStabilityPeriod = 5 * time.Second

var (
	zero32   int32 = 0
	falsePtr *bool = func(b bool) *bool { return &b }(false)
	truePtr  *bool = func(b bool) *bool { return &b }(true)
	userID   int64 = 65534
	groupID  int64 = 65534

	telegrafIntervalSeconds      = 10
	telegrafFlushIntervalSeconds = 10
	telegrafFlushJitterSeconds   = 5

	K6Image       string = "ghcr.io/reviewsignal/orderly-ape/k6:latest"
	TelegrafImage string = "telegraf:1.36.2-alpine"
)

// TestRunReconciler reconciles a TestScenario object
type TestRunReconciler struct {
	client.Client
	ClientGetter          func(address, token, organization string) InfluxDBClient
	Scheme                *runtime.Scheme
	GrafanaClient         GrafanaClient
	GrafanaURL            string
	VictoriaLogsURL       string
	VictoriaLogsAccountID int32
}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=testscenarios,verbs=get;list;watch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=influxdbs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

//nolint:gocyclo // Complex reconciliation logic requires this complexity
func (r *TestRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error

	// Fetch the TestRun instance
	testrun := &apev1.TestRun{}
	if err = r.Get(ctx, req.NamespacedName, testrun); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	phase := testrun.Phase()
	l := logf.FromContext(ctx, "state", testrun.Spec.State, "phase", phase)
	hctx := logf.IntoContext(ctx, l)

	if !testrun.DeletionTimestamp.IsZero() {
		return r.handleDelete(ctx, testrun)
	}

	switch phase {
	case apev1.TestRunPhaseCompleted, apev1.TestRunPhaseCanceled, apev1.TestRunPhaseFailed:
		return r.handleFinalPhase(hctx, testrun)
	case apev1.TestRunPhaseCanceling:
		return r.handleCancelation(hctx, testrun)
	case apev1.TestRunPhaseReady:
		return r.handleReadyPhase(hctx, testrun)
	case apev1.TestRunPhaseUnknown, apev1.TestRunPhasePending:
		return r.handleSetup(hctx, testrun)
	default:
		return ctrl.Result{}, nil
	}
}

func (r *TestRunReconciler) handleSetup(ctx context.Context, testrun *apev1.TestRun) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	l.V(1).Info("TestRun setup")

	var err error

	// Get the TestScenario
	scenario := &apev1.TestScenario{}
	scenarioKey := testrun.Spec.Scenario.AsNamespacedName()
	if err = r.Get(ctx, scenarioKey, scenario); err != nil {
		l.Error(err, "failed to get TestScenario")
		return ctrl.Result{}, err
	}

	changed := controllerutil.AddFinalizer(testrun, testRunFinalizerInfluxDBCredentials)

	if len(testrun.Status.WorkerStatuses) > len(testrun.Spec.Workers) {
		return ctrl.Result{}, fmt.Errorf("invalid TestRun status: more worker statuses than spec workers")
	}

	// Copy Workers from TestScenario if empty
	if len(testrun.Spec.Workers) == 0 && len(scenario.Spec.Workers) > 0 {
		testrun.Spec.Workers = make([]apev1.TestRunWorkerSpec, len(scenario.Spec.Workers))
		copy(testrun.Spec.Workers, scenario.Spec.Workers)
		changed = true
	}

	mergedEnvVars := r.mergeEnvVars(scenario.Spec.EnvVars, testrun.Spec.EnvVars)
	if !envVarsEqual(testrun.Spec.EnvVars, mergedEnvVars) {
		testrun.Spec.EnvVars = mergedEnvVars
		changed = true
	}

	mergedLabels := r.mergeLabels(scenario.Spec.Labels, testrun.Spec.Labels)
	if !maps.Equal(testrun.Spec.Labels, mergedLabels) {
		testrun.Spec.Labels = mergedLabels
		changed = true
	}

	// Update spec only if there were changes
	if changed {
		return ctrl.Result{}, r.Update(ctx, testrun)
	}

	orig := testrun.DeepCopy()
	changedStatus := false

	l.V(1).Info("TestRun initialize status")
	// Ensure InfluxDB secret is created
	if testrun.Status.InfluxDBSecret == nil {
		if err = r.ensureInfluxDBSecret(ctx, testrun); err != nil {
			l.Error(err, "failed to ensure InfluxDB secret")
			return ctrl.Result{}, err
		}
		changedStatus = true
	}

	if testrun.Status.ScenarioSpec == nil {
		testrun.Status.ScenarioSpec = &apev1.TestRunScenarioSpec{
			TestSource:  scenario.Spec.TestSource,
			Resources:   scenario.Spec.Resources,
			MaxDuration: scenario.Spec.MaxDuration,
		}
		changedStatus = true
	}

	if r.VictoriaLogsURL != "" && testrun.Status.VictoriaLogs == nil {
		if err = r.ensureVictoriaLogsConfig(ctx, testrun); err != nil {
			l.Error(err, "failed to ensure VictoriaLogs config")
			return ctrl.Result{}, err
		}
		changedStatus = true
	}

	if r.assignSegmentsToWorkers(testrun) {
		changedStatus = true
	}

	if changedStatus {
		return ctrl.Result{}, r.Status().Patch(ctx, testrun, client.MergeFrom(orig))
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) handleCancelation(ctx context.Context, testrun *apev1.TestRun) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	l.V(1).Info("TestRun cancelation")

	orig := testrun.DeepCopy()

	if _, err := r.cleanupCredentials(ctx, testrun); err != nil {
		return ctrl.Result{}, err
	}

	if testrun.Spec.State != apev1.TestRunStateCanceled {
		testrun.Spec.State = apev1.TestRunStateCanceled
		return ctrl.Result{}, r.Patch(ctx, testrun, client.MergeFrom(orig))
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) handleFinalPhase(ctx context.Context, testrun *apev1.TestRun) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	l.V(1).Info("TestRun finalization")

	orig := testrun.DeepCopy()
	needsUpdate := false

	// Set CompletionTime if not already set
	if testrun.Status.CompletionTime == nil {
		testrun.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		needsUpdate = true
	}

	if needsUpdate {
		if err := r.Status().Patch(ctx, testrun, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update the dashboard with the completion time.
	// Dashboard update failures are non-blocking because the test data is still recorded
	// and accessible, and dashboard issues shouldn't prevent test finalization.
	if err := r.updateGrafanaDashboard(ctx, testrun); err != nil {
		l.Error(err, "failed to update dashboard with completion time")
		// Don't fail reconciliation if dashboard update fails
	}

	if _, err := r.cleanupCredentials(ctx, testrun); err != nil {
		return ctrl.Result{}, err
	}

	if testrun.Spec.State != apev1.TestRunStateCanceled {
		testrun.Spec.State = apev1.TestRunStateArchived
		return ctrl.Result{}, r.Patch(ctx, testrun, client.MergeFrom(orig))
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) handleReadyPhase(ctx context.Context, testrun *apev1.TestRun) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	l.V(1).Info("TestRun ready phase")

	needsUpdate := false
	orig := testrun.DeepCopy()

	if testrun.Status.StartTime == nil {
		testrun.Status.StartTime = &metav1.Time{Time: time.Now().Add(time.Minute)}
		needsUpdate = true
	}

	if testrun.Status.DashboardUID == "" {
		uid, err := r.createGrafanaDashboard(ctx, testrun)
		if err != nil {
			return ctrl.Result{}, err
		}
		testrun.Status.DashboardUID = uid
		needsUpdate = true
	}

	if needsUpdate {
		return ctrl.Result{}, r.Status().Patch(ctx, testrun, client.MergeFrom(orig))
	}

	return ctrl.Result{}, nil
}

func (r *TestRunReconciler) handleDelete(ctx context.Context, testrun *apev1.TestRun) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	l.V(1).Info("TestRun deletion")
	return r.cleanupCredentials(ctx, testrun)
}

func workerStatusByName(name string, workers []apev1.TestRunWorkerStatus) *apev1.TestRunWorkerStatus {
	for i := range workers {
		if workers[i].Name == name {
			return &workers[i]
		}
	}
	return nil
}

func workerSpecByName(name string, workers []apev1.TestRunWorkerSpec) *apev1.TestRunWorkerSpec {
	for i := range workers {
		if workers[i].Name == name {
			return &workers[i]
		}
	}
	return nil
}

// assignSegmentsToWorkers assigns segments to workers based on their NumWorkers.
// It updates testrun.Status.WorkerStatuses[*].AssignedSegments with the assigned segments.
// Returns true if segments were assigned, false otherwise.
func (r *TestRunReconciler) assignSegmentsToWorkers(testrun *apev1.TestRun) bool {
	changed := false
	segments := testrun.Segments()
	randIdx := rand.Perm(len(segments))

	idx := 0
	for _, specWorker := range testrun.Spec.Workers {
		// Find or create the corresponding status entry
		var status *apev1.TestRunWorkerStatus
		for i := range testrun.Status.WorkerStatuses {
			if testrun.Status.WorkerStatuses[i].Name == specWorker.Name {
				status = &testrun.Status.WorkerStatuses[i]
				break
			}
		}
		if status == nil {
			testrun.Status.WorkerStatuses = append(testrun.Status.WorkerStatuses, apev1.TestRunWorkerStatus{Name: specWorker.Name, Phase: apev1.TestRunPhasePending})
			status = &testrun.Status.WorkerStatuses[len(testrun.Status.WorkerStatuses)-1]
		}

		if len(status.AssignedSegments) > 0 {
			continue
		}

		status.AssignedSegments = []string{}
		for i := int32(0); i < specWorker.NumWorkers && idx < len(segments); i++ {
			j := randIdx[idx]
			status.AssignedSegments = append(status.AssignedSegments, segments[j])
			changed = true
			idx++
		}
	}

	return changed
}

// mergeEnvVars merges environment variables, with priority given to the second list
func (r *TestRunReconciler) mergeEnvVars(base, override []apev1.EnvVar) []apev1.EnvVar {
	envVarMap := make(map[string]string)

	for _, env := range base {
		envVarMap[env.Name] = env.Value
	}

	for _, env := range override {
		envVarMap[env.Name] = env.Value
	}

	result := make([]apev1.EnvVar, 0, len(envVarMap))
	for name, value := range envVarMap {
		result = append(result, apev1.EnvVar{Name: name, Value: value})
	}

	return result
}

func cmpEnvVar(i, j apev1.EnvVar) int {
	keyCmp := strings.Compare(i.Name, j.Name)
	if keyCmp != 0 {
		return keyCmp
	}
	return strings.Compare(i.Value, j.Value)
}

func eqEnvVar(i, j apev1.EnvVar) bool {
	return cmpEnvVar(i, j) == 0
}

// envVarsEqual checks if two EnvVar slices are equal
func envVarsEqual(a, b []apev1.EnvVar) bool {
	sortedA := slices.SortedStableFunc(slices.Values(a), cmpEnvVar)
	sortedB := slices.SortedStableFunc(slices.Values(b), cmpEnvVar)
	return slices.EqualFunc(sortedA, sortedB, eqEnvVar)
}

// mergeLabels merges labels, with priority given to the second map
func (r *TestRunReconciler) mergeLabels(base, override map[string]string) map[string]string {
	result := make(map[string]string)
	maps.Copy(result, base)
	maps.Copy(result, override)

	return result
}

// ensureInfluxDBSecret ensures that the InfluxDB secret is created
func (r *TestRunReconciler) ensureInfluxDBSecret(ctx context.Context, testRun *apev1.TestRun) error {
	log := logf.FromContext(ctx)

	// Get the InfluxDB instance
	influxDB := &apev1.InfluxDB{}
	influxDBKey := testRun.Spec.InfluxDB.AsNamespacedName()
	if err := r.Get(ctx, influxDBKey, influxDB); err != nil {
		return fmt.Errorf("failed to get InfluxDB: %w", err)
	}

	// Get InfluxDB connection details
	address, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Address)
	if err != nil {
		return fmt.Errorf("failed to get InfluxDB address: %w", err)
	}

	token, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Token)
	if err != nil {
		return fmt.Errorf("failed to get InfluxDB token: %w", err)
	}

	organization, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Organization)
	if err != nil {
		return fmt.Errorf("failed to get InfluxDB organization: %w", err)
	}

	// Determine the bucket name
	bucket := "default"
	if testRun.Spec.InfluxDBBucket != nil && *testRun.Spec.InfluxDBBucket != "" {
		bucket = *testRun.Spec.InfluxDBBucket
	}

	// Create InfluxDB client
	clientGetter := r.ClientGetter
	if clientGetter == nil {
		clientGetter = NewInfluxDBClient
	}
	influxClient := clientGetter(address, token, organization)

	// Create a token with write permissions to the bucket
	orgID := influxDB.Status.OrganizationID
	description := fmt.Sprintf("Orderly Ape TestRun %s/%s", testRun.Namespace, testRun.Name)
	tokenID, tokenValue, err := influxClient.CreateToken(ctx, orgID, description, bucket)
	if err != nil {
		return fmt.Errorf("failed to create InfluxDB token: %w", err)
	}

	// Create the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: testRun.Name + "-influxdb-",
			Namespace:    testRun.Namespace,
			Annotations: map[string]string{
				"ape.reviewsignal.com/influxdb-token-id": tokenID,
			},
		},
		StringData: map[string]string{
			"INFLUXDB_HOST":   address,
			"INFLUXDB_ORG":    organization,
			"INFLUXDB_BUCKET": bucket,
			"INFLUXDB_TOKEN":  tokenValue,
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Set TestRun as the owner of the secret
	if err := controllerutil.SetControllerReference(testRun, secret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Create the secret
	if err := r.Create(ctx, secret); err != nil {
		// If creation failed, delete the token
		if delErr := influxClient.DeleteToken(ctx, tokenID); delErr != nil {
			log.Error(delErr, "failed to delete token after secret creation failure", "tokenID", tokenID)
		}
		return fmt.Errorf("failed to create secret: %w", err)
	}

	// Update TestRun status with the secret reference
	testRun.Status.InfluxDBSecret = &apev1.ObjectReference{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}

	return nil
}

// cleanupCredentials cleans up the InfluxDB credentials
//
//nolint:unparam // Result return kept for consistency with other handlers
func (r *TestRunReconciler) cleanupCredentials(ctx context.Context, testrun *apev1.TestRun) (ctrl.Result, error) {
	l := logf.FromContext(ctx)

	if testrun.Status.InfluxDBSecret != nil {
		// Get the secret
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      testrun.Status.InfluxDBSecret.Name,
			Namespace: testrun.Status.InfluxDBSecret.Namespace,
		}

		err := r.Get(ctx, secretKey, secret)
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get secret: %w", err)
		} else if err == nil { // Secret exists
			// Get token ID from secret annotations
			tokenID := secret.Annotations["ape.reviewsignal.com/influxdb-token-id"]
			if tokenID != "" {
				// Get the InfluxDB instance
				influxDB := &apev1.InfluxDB{}
				influxDBKey := testrun.Spec.InfluxDB.AsNamespacedName()
				if err := r.Get(ctx, influxDBKey, influxDB); err != nil {
					l.Error(err, "failed to get InfluxDB for token deletion")
				} else {
					// Get InfluxDB connection details
					address, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Address)
					if err != nil {
						return ctrl.Result{}, err
					}

					token, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Token)
					if err != nil {
						return ctrl.Result{}, err
					}
					organization, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Organization)
					if err != nil {
						return ctrl.Result{}, err
					}
					// Create InfluxDB client and delete the token
					clientGetter := r.ClientGetter
					if clientGetter == nil {
						clientGetter = NewInfluxDBClient
					}
					influxClient := clientGetter(address, token, organization)
					if err := influxClient.DeleteToken(ctx, tokenID); err != nil {
						return ctrl.Result{}, err
					}
				}
			}

			// Delete the secret
			l.V(1).Info("Deleting InfluxDB secret", "secret", secretKey)
			if err := r.Delete(ctx, secret); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete secret: %w", err)
			}
		}

		testrun.Status.InfluxDBSecret = nil
		if err := r.Status().Update(ctx, testrun); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update TestRun status: %w", err)
		}
	}

	// Remove credentials finalizer
	if controllerutil.RemoveFinalizer(testrun, testRunFinalizerInfluxDBCredentials) {
		return ctrl.Result{}, r.Update(ctx, testrun)
	}

	return ctrl.Result{}, nil
}

func getJobCondition(job *batchv1.Job, conditionType batchv1.JobConditionType) *batchv1.JobCondition {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func isPodStableReady(pod *corev1.Pod) bool {
	now := time.Now()

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.LastTransitionTime.Add(podReadyStabilityPeriod).Before(now) {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isDraft(testrun *apev1.TestRun) bool {
	return testrun.Spec.State == "Draft"
}

func isArchived(testrun *apev1.TestRun) bool {
	return testrun.Spec.State == "Archived"
}

func isCancelCompleted(testrun *apev1.TestRun) bool {
	if testrun.Spec.State != apev1.TestRunStateCanceled {
		return false
	}

	for _, ws := range testrun.Status.WorkerStatuses {
		if ws.Phase != apev1.TestRunPhaseCompleted && ws.Phase != apev1.TestRunPhaseFailed && ws.Phase != apev1.TestRunPhaseCanceled {
			return false
		}
	}
	return true
}

func skipDraft() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		testrun, ok := object.(*apev1.TestRun)
		return ok && !isDraft(testrun)
	})
}

func skipArchived() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		testrun, ok := object.(*apev1.TestRun)
		return ok && (!testrun.DeletionTimestamp.IsZero() || !isArchived(testrun))
	})
}

// skipCancelCompleted skips reconciliation for TestRuns that are canceled and all workers are in a final phase.
func skipCancelCompleted() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		testrun, ok := object.(*apev1.TestRun)
		return ok && (!testrun.DeletionTimestamp.IsZero() || !isCancelCompleted(testrun))
	})
}

// getInfluxDBBucket returns the InfluxDB bucket name for the test run.
// If TestRun.Spec.InfluxDBBucket is specified, it returns that value.
// Otherwise, it falls back to "default" as the bucket name.
func (r *TestRunReconciler) getInfluxDBBucket(testrun *apev1.TestRun) string {
	bucket := "default"
	if testrun.Spec.InfluxDBBucket != nil && *testrun.Spec.InfluxDBBucket != "" {
		bucket = *testrun.Spec.InfluxDBBucket
	}
	return bucket
}

// createGrafanaDashboard creates a Grafana dashboard for the test run
func (r *TestRunReconciler) createGrafanaDashboard(ctx context.Context, testrun *apev1.TestRun) (string, error) {
	l := logf.FromContext(ctx)

	// Get the InfluxDB resource to retrieve the datasource UID
	influxDB := &apev1.InfluxDB{}
	influxDBKey := testrun.Spec.InfluxDB.AsNamespacedName()
	if err := r.Get(ctx, influxDBKey, influxDB); err != nil {
		return "", fmt.Errorf("failed to get InfluxDB: %w", err)
	}

	// Get datasource UID from annotations
	dsInfluxUID, ok := influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasourceUID]
	if !ok || dsInfluxUID == "" {
		return "", fmt.Errorf("InfluxDB datasource UID not found in annotations")
	}

	dsVictoriaUID := ""
	if testrun.Status.VictoriaLogs != nil {
		dsVictoriaUID = safename.FromHostname(fmt.Sprintf("victorialogs-%d", testrun.Status.VictoriaLogs.AccountID), 31, testrun.Status.VictoriaLogs.URL)
	}

	// Get the bucket name
	bucket := r.getInfluxDBBucket(testrun)

	// Prepare the dashboard from the template
	var endTime *time.Time
	if testrun.Status.CompletionTime != nil {
		t := testrun.Status.CompletionTime.Time
		endTime = &t
	}
	startTime := &testrun.Status.StartTime.Time

	// Build tags array
	tags := r.buildDashboardTags(testrun)

	dashboard, err := grafana.PrepareDashboard(testrun.Name, testrun.Spec.Target, bucket, dsInfluxUID, dsVictoriaUID, info.Version.GitVersion, startTime, endTime, telegrafIntervalSeconds, tags)
	if err != nil {
		return "", fmt.Errorf("failed to prepare dashboard: %w", err)
	}

	// Create the dashboard in Grafana
	uid, err := r.GrafanaClient.CreateDashboard(ctx, dashboard)
	if err != nil {
		return "", fmt.Errorf("failed to create dashboard in Grafana: %w", err)
	}

	l.Info("Created Grafana dashboard", "uid", uid)

	return uid, nil
}

// updateGrafanaDashboard updates an existing Grafana dashboard for the test run
func (r *TestRunReconciler) updateGrafanaDashboard(ctx context.Context, testrun *apev1.TestRun) error {
	l := logf.FromContext(ctx)

	if testrun.Status.DashboardUID == "" {
		return nil // No dashboard to update
	}

	// Get the InfluxDB resource to retrieve the datasource UID
	influxDB := &apev1.InfluxDB{}
	influxDBKey := testrun.Spec.InfluxDB.AsNamespacedName()
	if err := r.Get(ctx, influxDBKey, influxDB); err != nil {
		return fmt.Errorf("failed to get InfluxDB: %w", err)
	}

	// Get datasource UID from annotations
	dsInfluxUID, ok := influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasourceUID]
	if !ok || dsInfluxUID == "" {
		return fmt.Errorf("InfluxDB datasource UID not found in annotations")
	}

	dsVictoriaUID := ""
	if testrun.Status.VictoriaLogs != nil {
		dsVictoriaUID = safename.FromHostname(fmt.Sprintf("victorialogs-%d", testrun.Status.VictoriaLogs.AccountID), 31, testrun.Status.VictoriaLogs.URL)
	}

	// Get the bucket name
	bucket := r.getInfluxDBBucket(testrun)

	// Prepare the dashboard from the template
	var endTime *time.Time
	if testrun.Status.CompletionTime != nil {
		t := testrun.Status.CompletionTime.Time
		endTime = &t
	}
	startTime := &testrun.Status.StartTime.Time

	// Build tags array
	tags := r.buildDashboardTags(testrun)

	dashboard, err := grafana.PrepareDashboard(testrun.Name, testrun.Spec.Target, bucket, dsInfluxUID, dsVictoriaUID, info.Version.GitVersion, startTime, endTime, telegrafIntervalSeconds, tags)
	if err != nil {
		return fmt.Errorf("failed to prepare dashboard: %w", err)
	}

	// Update the dashboard in Grafana
	err = r.GrafanaClient.UpdateDashboard(ctx, testrun.Status.DashboardUID, dashboard)
	if err != nil {
		return fmt.Errorf("failed to update dashboard in Grafana: %w", err)
	}

	l.Info("Updated Grafana dashboard", "uid", testrun.Status.DashboardUID)

	return nil
}

// buildDashboardTags builds the tags array for the dashboard from the test run
func (r *TestRunReconciler) buildDashboardTags(testrun *apev1.TestRun) []string {
	tags := []string{}

	// Add orderly-ape version tag
	tags = append(tags, fmt.Sprintf("orderly-ape/%s", info.Version.GitVersion))

	// Add scenario tag
	if testrun.Spec.Scenario != nil {
		tags = append(tags, fmt.Sprintf("scenario/%s", testrun.Spec.Scenario.Name))
	}

	// Add label tags (KEY/VALUE format, no label/ prefix)
	for key, value := range testrun.Spec.Labels {
		tags = append(tags, fmt.Sprintf("%s/%s", key, value))
	}

	return tags
}

// ensureVictoriaLogsConfig ensures VictoriaLogs configuration is set up for the test run
func (r *TestRunReconciler) ensureVictoriaLogsConfig(ctx context.Context, testrun *apev1.TestRun) error {
	l := logf.FromContext(ctx)

	// Generate a random project ID
	projectID := rand.Int32()

	// Set VictoriaLogs configuration in status
	testrun.Status.VictoriaLogs = &apev1.VictoriaLogsConfig{
		URL:       r.VictoriaLogsURL,
		AccountID: r.VictoriaLogsAccountID,
		ProjectID: projectID,
	}

	// Ensure Grafana datasource exists if Grafana is configured
	if r.GrafanaClient != nil {
		if err := r.ensureVictoriaLogsGrafanaDatasource(ctx, testrun); err != nil {
			l.Error(err, "failed to ensure VictoriaLogs Grafana datasource")
			return fmt.Errorf("failed to ensure VictoriaLogs Grafana datasource: %w", err)
		}
	}

	return nil
}

// ensureVictoriaLogsGrafanaDatasource ensures a Grafana datasource exists for VictoriaLogs
func (r *TestRunReconciler) ensureVictoriaLogsGrafanaDatasource(ctx context.Context, testrun *apev1.TestRun) error {
	l := logf.FromContext(ctx)

	// Generate a deterministic UID for the datasource based on VictoriaLogs URL
	dsName := safename.FromHostname(fmt.Sprintf("victorialogs-%d", testrun.Status.VictoriaLogs.AccountID), 31, testrun.Status.VictoriaLogs.URL)

	// Check if datasource already exists
	_, err := r.GrafanaClient.GetDatasource(ctx, dsName)
	if err == nil {
		// Datasource already exists
		l.V(1).Info("VictoriaLogs Grafana datasource already exists", "name", dsName)
		return nil
	}
	if err != ErrNotFound {
		return fmt.Errorf("failed to check for existing datasource: %w", err)
	}

	// Create the datasource
	ds := &GrafanaDatasource{
		Name:     dsName,
		UID:      dsName,
		TypeName: "VictoriaLogs",
		Type:     "victoriametrics-logs-datasource",
		URL:      testrun.Status.VictoriaLogs.URL,
		Access:   "proxy",
		JSONData: map[string]any{
			"accountID": testrun.Status.VictoriaLogs.AccountID,
		},
	}

	uid, err := r.GrafanaClient.CreateDatasource(ctx, ds)
	if err != nil {
		return fmt.Errorf("failed to create VictoriaLogs datasource in Grafana: %w", err)
	}

	l.Info("Created VictoriaLogs Grafana datasource", "uid", uid)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apev1.TestRun{}, builder.WithPredicates(skipDraft(), skipArchived(), skipCancelCompleted())).
		Named("testrun").
		Complete(r)
}
