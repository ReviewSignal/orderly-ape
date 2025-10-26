//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2025 ReviewSignal

// Package worker provides a Kubernetes cluster provider that watches Worker
// resources and creates controller-runtime clusters for each.
package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/util/common"
)

var _ multicluster.Provider = &Provider{}

const (
	// testrunJobRoleName is the name of the ClusterRole created for testrun jobs
	testrunJobRoleName = "orderly-ape-testrun-job-role"
)

// New creates a new Worker Provider.
func New(opts Options) *Provider {
	return &Provider{
		opts:     opts,
		log:      logf.Log.WithName("worker-provider"),
		clusters: map[string]activeCluster{},
	}
}

// Options contains the configuration for the worker provider.
type Options struct {
	// Namespace is the namespace where Worker resources are stored in the local cluster.
	Namespace string
	// DefaultWorkerNamespace is the default namespace to watch in worker clusters if not specified in Worker spec.
	DefaultWorkerNamespace string
	// ClusterOptions is the list of options to pass to the cluster object.
	ClusterOptions []cluster.Option
	// RESTOptions is the list of options to pass to the rest client.
	RESTOptions []func(cfg *rest.Config) error
}

type index struct {
	object       client.Object
	field        string
	extractValue client.IndexerFunc
}

// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ape.reviewsignal.com,resources=workers/finalizers,verbs=update

// Provider is a cluster provider that watches for Worker resources
// and engages clusters based on those workers.
type Provider struct {
	opts     Options
	log      logr.Logger
	lock     sync.RWMutex // protects clusters and indexers
	clusters map[string]activeCluster
	indexers []index
	mgr      mcmanager.Manager
}

type activeCluster struct {
	Cluster   cluster.Cluster
	Context   context.Context
	Cancel    context.CancelFunc
	Hash      string // hash of the kubeconfig
	Namespace string // namespace to watch in the worker cluster
}

// getCluster retrieves a cluster by name with read lock
func (p *Provider) getCluster(clusterName string) (activeCluster, bool) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	ac, exists := p.clusters[clusterName]
	return ac, exists
}

// setCluster adds a cluster with write lock
func (p *Provider) setCluster(clusterName string, ac activeCluster) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.clusters[clusterName] = ac
}

// addIndexer adds an indexer with write lock
func (p *Provider) addIndexer(idx index) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.indexers = append(p.indexers, idx)
}

// Get returns the cluster with the given name, if it is known.
func (p *Provider) Get(ctx context.Context, clusterName string) (cluster.Cluster, error) {
	ac, exists := p.getCluster(clusterName)
	if !exists {
		return nil, multicluster.ErrClusterNotFound
	}
	return ac.Cluster, nil
}

// SetupWithManager sets up the provider with the manager.
func (p *Provider) SetupWithManager(ctx context.Context, mgr mcmanager.Manager) error {
	log := p.log
	log.Info("Starting worker provider")

	if mgr == nil {
		return fmt.Errorf("manager is nil")
	}
	p.mgr = mgr

	// Get the local manager from the multicluster manager
	localMgr := mgr.GetLocalManager()
	if localMgr == nil {
		return fmt.Errorf("local manager is nil")
	}

	// Index the spec.kubeconfigSecret field for efficient lookups
	if err := localMgr.GetFieldIndexer().IndexField(ctx, &apev1.Worker{}, "spec.kubeconfigSecret", func(o client.Object) []string {
		worker := o.(*apev1.Worker)
		if worker.Spec.KubeconfigSecret == nil {
			return nil
		}

		namespace := worker.Spec.KubeconfigSecret.Namespace
		if namespace == "" {
			namespace = worker.Namespace
		}
		namespacedName := namespace + "/" + worker.Spec.KubeconfigSecret.Name
		return []string{namespacedName}
	}); err != nil {
		return fmt.Errorf("failed to index spec.kubeconfigSecret: %w", err)
	}

	// Setup the controller to watch for Worker resources
	err := ctrl.NewControllerManagedBy(localMgr).
		For(&apev1.Worker{}, builder.WithPredicates(
			predicate.And(
				// Only watch for Workers in the configured namespace
				predicate.NewPredicateFuncs(func(obj client.Object) bool {
					return obj.GetNamespace() == p.opts.Namespace
				}),
				// Only trigger on generation changes (spec changes)
				predicate.GenerationChangedPredicate{},
			),
		)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
			_, ok := o.(*corev1.Secret)
			if !ok {
				return nil
			}
			secretKey := client.ObjectKeyFromObject(o).String()

			// Find all Workers that reference this secret
			workerList := &apev1.WorkerList{}
			if err := localMgr.GetClient().List(ctx, workerList,
				client.InNamespace(p.opts.Namespace),
				client.MatchingFields{"spec.kubeconfigSecret": secretKey},
			); err != nil {
				log.Error(err, "failed to list workers for secret", "secret", client.ObjectKeyFromObject(o))
				return nil
			}

			// Trigger reconciliation for each affected Worker
			requests := make([]reconcile.Request, 0, len(workerList.Items))
			for _, worker := range workerList.Items {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&worker)})
			}

			return requests
		})).
		Complete(p)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	return nil
}

// Reconcile is the main controller function that reconciles Worker resources
func (p *Provider) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Handle Worker retrieval and basic validation
	worker, err := p.getWorker(ctx, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if worker == nil {
		// Worker not found, remove cluster if it exists
		p.removeCluster(req.String())
		return ctrl.Result{}, nil
	}

	// Extract cluster name and create logger
	clusterName := req.String()
	log := p.log.WithValues("cluster", clusterName, "worker", fmt.Sprintf("%s/%s", worker.Namespace, worker.Name))

	// Handle Worker deletion, this is usually only hit if there is a finalizer on the Worker.
	if worker.DeletionTimestamp != nil {
		p.removeCluster(clusterName)
		return ctrl.Result{}, nil
	}

	if len(worker.Status.Conditions) == 0 {
		worker.Status.Conditions = []metav1.Condition{}
	}

	// Extract and validate kubeconfig from Worker spec
	kubeconfigData, restConfig, err := p.getRestConfigFromWorker(ctx, worker)
	if err != nil {
		log.Error(err, "Failed to get kubeconfig from Worker")
		return ctrl.Result{}, p.updateWorkerStatus(ctx, worker, metav1.ConditionFalse, "InvalidKubeconfig", err.Error())
	}

	// Hash the kubeconfig for change detection (or use empty string if using local config)
	var hashStr string
	if kubeconfigData != nil {
		hashStr = p.hashKubeconfig(kubeconfigData)
	}

	// Get namespace from Worker spec, use default if not specified
	workerNamespace := worker.Spec.Namespace
	if workerNamespace == "" {
		workerNamespace = p.opts.DefaultWorkerNamespace
	}
	if workerNamespace == "" {
		log.Info("Worker does not have namespace configured and no default is set, skipping")
		return ctrl.Result{}, p.updateWorkerStatus(ctx, worker, metav1.ConditionFalse, "MissingWorkerNamespace",
			"Worker namespace not configured and no default set")
	}

	// Check if cluster exists and needs to be updated
	existingCluster, clusterExists := p.getCluster(clusterName)
	if clusterExists && existingCluster.Hash == hashStr && existingCluster.Namespace == workerNamespace {
		log.Info("Cluster already exists with same kubeconfig and namespace, requeuing next check")
		ready := meta.FindStatusCondition(worker.Status.Conditions, apev1.WorkerConditionReady)
		if ready == nil || ready.Status != metav1.ConditionTrue {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	} else {
		if clusterExists {
			// If the cluster exists and the kubeconfig or namespace has changed,
			// remove it and continue to create a new cluster in its place.
			log.Info("Cluster already exists, updating it")
			p.removeCluster(clusterName)
		}

		log.Info("Setting initial Worker status condition")
		if err := p.updateWorkerStatus(ctx, worker, metav1.ConditionFalse, "Initializing", "Worker is being initialized"); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := p.updateWorkerStatus(ctx, worker, metav1.ConditionFalse, "ClusterStarting", "Starting cluster connection"); err != nil {
		return ctrl.Result{}, err
	}

	// Create and setup the new cluster
	if err := p.createAndEngageCluster(ctx, clusterName, restConfig, hashStr, workerNamespace, log); err != nil {
		log.Error(err, "Failed to create and engage cluster")
		return ctrl.Result{}, p.updateWorkerStatus(ctx, worker, metav1.ConditionFalse, "EngageFailed", err.Error())
	}

	// Verify that we can create jobs in the worker cluster and update status
	if err := p.verifyAndUpdateWorkerStatus(ctx, worker, clusterName, workerNamespace, log); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// verifyAndUpdateWorkerStatus verifies job creation capability and updates worker status accordingly
func (p *Provider) verifyAndUpdateWorkerStatus(ctx context.Context, worker *apev1.Worker, clusterName, workerNamespace string, log logr.Logger) error {
	ac, exists := p.getCluster(clusterName)
	if !exists {
		err := fmt.Errorf("cluster not found after engagement")
		log.Error(err, "Cluster not found after engagement")
		return p.updateWorkerStatus(ctx, worker, metav1.ConditionFalse, "ClusterNotFound", err.Error())
	}

	if err := p.verifyJobCreation(ctx, ac.Cluster, workerNamespace); err != nil {
		log.Error(err, "Failed to verify job creation in worker cluster")
		return p.updateWorkerStatus(ctx, worker, metav1.ConditionFalse, "JobCreationFailed",
			fmt.Sprintf("Cannot create jobs in worker cluster: %v", err))
	}

	// Setup RBAC resources for testrun jobs (non-fatal if it fails)
	p.setupRBAC(ctx, ac.Cluster, workerNamespace, log)

	// Set status to Ready
	return p.updateWorkerStatus(ctx, worker, metav1.ConditionTrue, "Ready", "Worker cluster is ready and can create jobs")
}

// updateWorkerStatus updates the Worker status condition and persists it
func (p *Provider) updateWorkerStatus(ctx context.Context, worker *apev1.Worker, status metav1.ConditionStatus, reason, message string) error {
	if meta.SetStatusCondition(&worker.Status.Conditions, metav1.Condition{
		Type:               apev1.WorkerConditionReady,
		Status:             status,
		ObservedGeneration: worker.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}) {
		if err := p.mgr.GetLocalManager().GetClient().Status().Update(ctx, worker); err != nil {
			return err
		}
	}
	return nil
}

// getWorker retrieves a Worker and handles not found errors
func (p *Provider) getWorker(ctx context.Context, namespacedName client.ObjectKey) (*apev1.Worker, error) {
	worker := &apev1.Worker{}
	if err := p.mgr.GetLocalManager().GetClient().Get(ctx, namespacedName, worker); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // Worker not found is not an error
		}
		return nil, fmt.Errorf("failed to get worker: %w", err)
	}
	return worker, nil
}

// getRestConfigFromWorker extracts rest config from the Worker's referenced secret or uses local config
func (p *Provider) getRestConfigFromWorker(ctx context.Context, worker *apev1.Worker) ([]byte, *rest.Config, error) {
	// If no kubeconfig secret is specified, use local in-cluster config
	if worker.Spec.KubeconfigSecret == nil {
		cfg, err := ctrl.GetConfig()
		if err != nil {
			return nil, nil, err
		}
		return nil, rest.CopyConfig(cfg), err
	}

	secretNamespace := worker.Spec.KubeconfigSecret.Namespace
	if secretNamespace == "" {
		secretNamespace = worker.Namespace
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      worker.Spec.KubeconfigSecret.Name,
		Namespace: secretNamespace,
	}

	if err := p.mgr.GetLocalManager().GetClient().Get(ctx, secretKey, secret); err != nil {
		return nil, nil, fmt.Errorf("failed to get kubeconfig secret: %w", err)
	}

	kubeconfigData, ok := secret.Data["kubeconfig"]
	if !ok || len(kubeconfigData) == 0 {
		return nil, nil, fmt.Errorf("kubeconfig key not found in secret %s", secretKey)
	}

	// Parse the kubeconfig
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	return kubeconfigData, restConfig, nil
}

// hashKubeconfig creates a hash of the kubeconfig data
func (p *Provider) hashKubeconfig(kubeconfigData []byte) string {
	if len(kubeconfigData) == 0 {
		return ""
	}
	hash := sha256.New()
	hash.Write(kubeconfigData)
	return hex.EncodeToString(hash.Sum(nil))
}

// createAndEngageCluster creates a new cluster, sets it up, stores it, and engages it with the manager
func (p *Provider) createAndEngageCluster(ctx context.Context, clusterName string, restConfig *rest.Config, hashStr, namespace string, log logr.Logger) error {
	// Apply REST options
	for _, opt := range p.opts.RESTOptions {
		if err := opt(restConfig); err != nil {
			return fmt.Errorf("failed to apply REST option: %w", err)
		}
	}

	// Create cluster options with namespace filter
	clusterOpts := append([]cluster.Option{}, p.opts.ClusterOptions...)
	clusterOpts = append(clusterOpts, func(opts *cluster.Options) {
		opts.Cache.DefaultNamespaces = map[string]cache.Config{
			namespace: {},
		}
	})

	// Create a new cluster
	log.Info("Creating new cluster from Worker", "namespace", namespace)
	cl, err := cluster.New(restConfig, clusterOpts...)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	// Apply field indexers
	if err := p.applyIndexers(ctx, cl); err != nil {
		return err
	}

	// Create a context that will be canceled when this cluster is removed
	clusterCtx, cancel := context.WithCancel(ctx)

	// Start the cluster
	go func() {
		if err := cl.Start(clusterCtx); err != nil {
			log.Error(err, "Failed to start cluster")
		}
	}()

	// Wait for cache to be ready
	log.Info("Waiting for cluster cache to be ready")
	if !cl.GetCache().WaitForCacheSync(clusterCtx) {
		cancel()
		return fmt.Errorf("failed to wait for cache sync")
	}
	log.Info("Cluster cache is ready")

	// Store the cluster
	p.setCluster(clusterName, activeCluster{
		Cluster:   cl,
		Context:   clusterCtx,
		Cancel:    cancel,
		Hash:      hashStr,
		Namespace: namespace,
	})

	log.Info("Successfully added cluster")

	// Engage cluster so that the manager can start operating on the cluster
	if err := p.mgr.Engage(clusterCtx, clusterName, cl); err != nil {
		log.Error(err, "Failed to engage manager, removing cluster")
		p.removeCluster(clusterName)
		return fmt.Errorf("failed to engage manager: %w", err)
	}

	log.Info("Successfully engaged manager", "namespace", namespace)
	return nil
}

// applyIndexers applies field indexers to a cluster
func (p *Provider) applyIndexers(ctx context.Context, cl cluster.Cluster) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, idx := range p.indexers {
		if err := cl.GetFieldIndexer().IndexField(ctx, idx.object, idx.field, idx.extractValue); err != nil {
			return fmt.Errorf("failed to index field %q: %w", idx.field, err)
		}
	}

	return nil
}

// IndexField indexes a field on all clusters, existing and future.
func (p *Provider) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	// Save for future clusters
	p.addIndexer(index{
		object:       obj,
		field:        field,
		extractValue: extractValue,
	})

	// Apply to existing clusters
	p.lock.RLock()
	defer p.lock.RUnlock()

	for name, ac := range p.clusters {
		if err := ac.Cluster.GetFieldIndexer().IndexField(ctx, obj, field, extractValue); err != nil {
			return fmt.Errorf("failed to index field %q on cluster %q: %w", field, name, err)
		}
	}

	return nil
}

// ListClusters returns a list of all discovered clusters.
func (p *Provider) ListClusters() []string {
	p.lock.RLock()
	defer p.lock.RUnlock()

	result := make([]string, 0, len(p.clusters))
	for name := range p.clusters {
		result = append(result, name)
	}
	return result
}

// removeCluster removes a cluster by name with write lock and cleanup
func (p *Provider) removeCluster(clusterName string) {
	log := p.log.WithValues("cluster", clusterName)

	p.lock.Lock()
	ac, exists := p.clusters[clusterName]
	if !exists {
		p.lock.Unlock()
		log.Info("Cluster not found, nothing to remove")
		return
	}

	log.Info("Removing cluster")
	delete(p.clusters, clusterName)
	p.lock.Unlock()

	// Cancel the context to trigger cleanup for this cluster.
	// This is done outside the lock to avoid holding the lock for a long time.
	ac.Cancel()
	log.Info("Successfully removed cluster and cancelled cluster context")
}

// verifyJobCreation verifies that we can create jobs in the worker cluster namespace.
// It first tries to create a paused dummy job. If the namespace doesn't exist, it creates
// the namespace and retries. Returns an error if job creation fails.
func (p *Provider) verifyJobCreation(ctx context.Context, cl cluster.Cluster, namespace string) error {
	// Try to create a dummy paused job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "orderly-ape-verify-",
			Namespace:    namespace,
			Labels: map[string]string{
				common.Label("job-verify"): "true",
			},
		},
		Spec: batchv1.JobSpec{
			Suspend: ptr.To(true), // Paused job - won't run
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name: "verify",
							// Kubernetes pause image, small and guaranteed to exist
							// sha256 of the 3.9 version
							Image: "registry.k8s.io/pause@sha256:7031c1b283388d2c2e09b57badb803c05ebed362dc88d84b480cc47f72a21097",
						},
					},
				},
			},
		},
	}

	// Try to create the job
	err := cl.GetClient().Create(ctx, job)
	if err == nil {
		// Job created successfully, clean it up
		p.log.Info("Job creation verification succeeded", "namespace", namespace)
		_ = cl.GetClient().Delete(ctx, job)
		return nil
	}

	// If the namespace doesn't exist, try to create it
	if apierrors.IsNotFound(err) {
		p.log.Info("Namespace not found, creating it", "namespace", namespace)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		if err := cl.GetClient().Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create namespace: %w", err)
		}

		// Retry job creation
		err = cl.GetClient().Create(ctx, job)
		if err == nil {
			p.log.Info("Job creation verification succeeded after creating namespace", "namespace", namespace)
			_ = cl.GetClient().Delete(ctx, job)
			return nil
		}
	}

	return fmt.Errorf("failed to create verification job: %w", err)
}

// setupRBAC creates ClusterRole and ClusterRoleBinding for testrun jobs.
// Failures are logged but not treated as errors.
func (p *Provider) setupRBAC(ctx context.Context, cl cluster.Cluster, namespace string, log logr.Logger) {
	// Create or update the ClusterRole
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: testrunJobRoleName,
			Labels: map[string]string{
				common.KubernetesAppName:      "orderly-ape",
				common.KubernetesAppManagedBy: "orderly-ape",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"metrics.k8s.io"},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes", "nodes/proxy", "nodes/stats", "nodes/pods", "persistentvolumes"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	if err := cl.GetClient().Create(ctx, clusterRole); err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.V(1).Info("ClusterRole already exists", "name", testrunJobRoleName)
		} else {
			log.Info("Failed to create ClusterRole (non-fatal)", "name", testrunJobRoleName, "error", err)
		}
	} else {
		log.Info("Created ClusterRole", "name", testrunJobRoleName)
	}

	// Create the ClusterRoleBinding for this namespace
	bindingName := namespace + "-testrun-job-role-binding"
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: bindingName,
			Labels: map[string]string{
				common.KubernetesAppName:      "orderly-ape",
				common.KubernetesAppManagedBy: "orderly-ape",
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "default",
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     testrunJobRoleName,
			APIGroup: rbacv1.GroupName,
		},
	}

	if err := cl.GetClient().Create(ctx, clusterRoleBinding); err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.V(1).Info("ClusterRoleBinding already exists", "name", bindingName)
		} else {
			log.Info("Failed to create ClusterRoleBinding (non-fatal)", "name", bindingName, "namespace", namespace, "error", err)
		}
	} else {
		log.Info("Created ClusterRoleBinding", "name", bindingName, "namespace", namespace)
	}
}
