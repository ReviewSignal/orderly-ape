// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	safename "github.com/ReviewSignal/orderly-ape/internal/util/name"
	"github.com/ReviewSignal/orderly-ape/internal/util/sourcedvalue"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

// InfluxDBReconciler reconciles a InfluxDB object
type InfluxDBReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientGetter  func(address, token, organization string) InfluxDBClient
	GrafanaClient GrafanaClient
	GrafanaOrgID  int
}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=influxdbs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=influxdbs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=influxdbs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the InfluxDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *InfluxDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the InfluxDB instance
	influxDB := &apev1.InfluxDB{}
	if err := r.Get(ctx, req.NamespacedName, influxDB); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the address, token, and organization from sourced values
	address, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Address)
	if err != nil {
		log.Error(err, "failed to get InfluxDB address")
		return r.updateStatusOnError(ctx, influxDB, apev1.InfluxDBReasonConnectionFailed, fmt.Sprintf("Failed to get address: %v", err))
	}

	token, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Token)
	if err != nil {
		log.Error(err, "failed to get InfluxDB token")
		return r.updateStatusOnError(ctx, influxDB, apev1.InfluxDBReasonConnectionFailed, fmt.Sprintf("Failed to get token: %v", err))
	}

	organization, err := sourcedvalue.Get(ctx, r.Client, influxDB.Namespace, influxDB.Spec.Organization)
	if err != nil {
		log.Error(err, "failed to get InfluxDB organization")
		return r.updateStatusOnError(ctx, influxDB, apev1.InfluxDBReasonConnectionFailed, fmt.Sprintf("Failed to get organization: %v", err))
	}

	// Create InfluxDB client
	clientGetter := r.ClientGetter
	if clientGetter == nil {
		clientGetter = NewInfluxDBClient
	}
	influxClient := clientGetter(address, token, organization)

	// Query /ready endpoint
	version, startTime, err := influxClient.GetReady(ctx)
	if err != nil {
		log.Error(err, "failed to query InfluxDB /ready endpoint")
		return r.updateStatusOnError(ctx, influxDB, apev1.InfluxDBReasonConnectionFailed, fmt.Sprintf("Failed to connect: %v", err))
	}

	// Get Org ID
	orgID, err := influxClient.GetOrganizationID(ctx, organization)
	if err != nil {
		log.Error(err, "failed to get InfluxDB organization ID")
		return r.updateStatusOnError(ctx, influxDB, apev1.InfluxDBReasonGetOrgIDFailed, fmt.Sprintf("Failed to get organization ID: %v", err))
	}

	// List buckets
	buckets, err := influxClient.ListBuckets(ctx)
	if err != nil {
		log.Error(err, "failed to list InfluxDB buckets")
		return r.updateStatusOnError(ctx, influxDB, apev1.InfluxDBReasonListBucketsFailed, fmt.Sprintf("Failed to list buckets: %v", err))
	}

	// Update status with success
	influxDB.Status.Version = version
	influxDB.Status.ServerStartTime = &metav1.Time{Time: startTime}
	influxDB.Status.OrganizationID = orgID
	influxDB.Status.Buckets = buckets

	// Create or update Grafana datasource if GrafanaClient is configured
	if r.GrafanaClient != nil {
		if err := r.reconcileGrafanaDatasource(ctx, influxDB, influxClient, address, organization, orgID); err != nil {
			log.Error(err, "failed to reconcile Grafana datasource")
			// Don't fail the entire reconciliation, just log the error
		}
	}

	meta.SetStatusCondition(&influxDB.Status.Conditions, metav1.Condition{
		Type:               apev1.InfluxDBConditionReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: influxDB.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             apev1.InfluxDBReasonReady,
		Message:            "InfluxDB server is ready",
	})

	if err := r.Status().Update(ctx, influxDB); err != nil {
		log.Error(err, "failed to update InfluxDB status")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute for periodic status sync
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// updateStatusOnError updates the status with error condition and clears ServerStartTime
func (r *InfluxDBReconciler) updateStatusOnError(ctx context.Context, influxDB *apev1.InfluxDB, reason, message string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Clear ServerStartTime on error
	influxDB.Status.ServerStartTime = nil

	meta.SetStatusCondition(&influxDB.Status.Conditions, metav1.Condition{
		Type:               apev1.InfluxDBConditionReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: influxDB.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, influxDB); err != nil {
		log.Error(err, "failed to update InfluxDB status")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute for periodic retry
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileGrafanaDatasource creates or updates a Grafana datasource for the InfluxDB connection
func (r *InfluxDBReconciler) reconcileGrafanaDatasource(ctx context.Context, influxDB *apev1.InfluxDB, influxClient InfluxDBClient, address, organization, orgID string) error {
	log := logf.FromContext(ctx)

	// Initialize annotations if needed
	if influxDB.Annotations == nil {
		influxDB.Annotations = make(map[string]string)
	}

	// Check for existing datasource name in annotations (authoritative)
	dsName, exists := influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasource]
	if !exists {
		// Generate datasource name using the safename utility
		dsName = safename.FromHostname("influxdb-"+influxDB.Name, 31, string(influxDB.UID))
		influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasource] = dsName
		if err := r.Update(ctx, influxDB); err != nil {
			return fmt.Errorf("failed to update datasource name annotation: %w", err)
		}
		log.Info("Set datasource name annotation", "name", dsName)
	}

	// Check for existing token ID in annotations (authoritative)
	tokenID, tokenExists := influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaReadTokenID]
	var tokenValue string
	var err error

	if !tokenExists || tokenID == "" {
		// Create a new read-only token
		tokenID, tokenValue, err = influxClient.CreateReadOnlyToken(ctx, orgID, fmt.Sprintf("Orderly Ape: Grafana datasource (%s)", dsName))
		if err != nil {
			return fmt.Errorf("failed to create read-only token: %w", err)
		}
		log.Info("Created read-only token for Grafana", "tokenID", tokenID)
		influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaReadTokenID] = tokenID
		if err := r.Update(ctx, influxDB); err != nil {
			return fmt.Errorf("failed to update token ID annotation: %w", err)
		}
	}

	// Check for existing datasource UID in annotations (authoritative)
	datasourceUID, dsUIDExists := influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasourceUID]
	var existingDS *GrafanaDatasource

	if dsUIDExists && datasourceUID != "" {
		existingDS, err = r.GrafanaClient.GetDatasource(ctx, datasourceUID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				log.Info("Datasource not found in Grafana, will recreate", "uid", datasourceUID)
				// Clear the UID annotation if datasource doesn't exist so we can recreate
				delete(influxDB.Annotations, apev1.InfluxDBAnnotationGrafanaDatasourceUID)
				if err := r.Update(ctx, influxDB); err != nil {
					return fmt.Errorf("failed to clear datasource UID annotation: %w", err)
				}
				existingDS = nil
			} else {
				log.Error(err, "failed to get existing datasource", "uid", datasourceUID)
				return fmt.Errorf("failed to get existing datasource: %w", err)
			}
		}
	}

	// Prepare datasource configuration
	ds := &GrafanaDatasource{
		Name:   dsName,
		Type:   "influxdb",
		URL:    address,
		Access: "proxy",
		OrgID:  r.GrafanaOrgID,
		JSONData: map[string]any{
			"version":       "Flux",
			"organization":  organization,
			"defaultBucket": "default",
			"tlsSkipVerify": false,
		},
	}

	// Only set the token if we just created it
	if tokenValue != "" {
		ds.SecureJSONData = map[string]string{
			"token": tokenValue,
		}
	}

	// Create or update the datasource
	if existingDS == nil {
		// Create new datasource
		uid, err := r.GrafanaClient.CreateDatasource(ctx, ds)
		if err != nil {
			return fmt.Errorf("failed to create Grafana datasource: %w", err)
		}
		log.Info("Created Grafana datasource", "uid", uid, "name", dsName)
		influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasourceUID] = uid
		if err := r.Update(ctx, influxDB); err != nil {
			return fmt.Errorf("failed to update datasource UID annotation: %w", err)
		}
	} else if tokenValue != "" {
		// Update existing datasource only if we have a new token
		if err := r.GrafanaClient.UpdateDatasource(ctx, datasourceUID, ds); err != nil {
			return fmt.Errorf("failed to update Grafana datasource: %w", err)
		}
		log.Info("Updated Grafana datasource", "uid", datasourceUID, "name", dsName)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InfluxDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apev1.InfluxDB{}).
		Named("influxdb").
		Complete(r)
}
