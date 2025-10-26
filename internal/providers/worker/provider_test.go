// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package worker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcluster "sigs.k8s.io/multicluster-runtime/pkg/multicluster"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const workerNamespace = "testing"
const workerClusterNamespace = "default"

var _ = Describe("Provider", Ordered, func() {
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	var provider *Provider
	var mgr mcmanager.Manager
	var localCli client.Client
	var workerCli client.Client

	BeforeAll(func() {
		var err error

		// Add Worker scheme to the default scheme
		err = apev1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		localCli, err = client.New(localCfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		workerCli, err = client.New(workerCfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		provider = New(Options{
			Namespace:              workerNamespace,
			DefaultWorkerNamespace: workerClusterNamespace,
			RESTOptions: []func(cfg *rest.Config) error{
				func(cfg *rest.Config) error {
					cfg.QPS = 100
					cfg.Burst = 200
					return nil
				},
			},
			ClusterOptions: []cluster.Option{
				func(clusterOptions *cluster.Options) {
					clusterOptions.Scheme = scheme.Scheme
				},
			},
		})

		By("Creating a namespace in the local cluster", func() {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workerNamespace,
				},
			}

			err = localCli.Create(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating kubeconfig secret in the local cluster", func() {
			err = createKubeconfigSecret(ctx, "worker-secret", workerCfg, workerNamespace, localCli)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating Worker resource in the local cluster", func() {
			worker := &apev1.Worker{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-worker",
					Namespace: workerNamespace,
				},
				Spec: apev1.WorkerSpec{
					KubeconfigSecret: &corev1.SecretReference{
						Name:      "worker-secret",
						Namespace: workerNamespace,
					},
					Namespace: workerClusterNamespace,
				},
			}
			err = localCli.Create(ctx, worker)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Setting up the cluster-aware manager, with the provider to lookup clusters", func() {
			var err error
			mgr, err = mcmanager.New(localCfg, provider, mcmanager.Options{
				Metrics: metricsserver.Options{
					BindAddress: "0",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		By("Setting up the provider with the manager", func() {
			err := provider.SetupWithManager(ctx, mgr)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Setting up the controller for ConfigMaps", func() {
			err := mcbuilder.ControllerManagedBy(mgr).
				Named("worker-configmap-controller").
				For(&corev1.ConfigMap{}).
				Complete(mcreconcile.Func(
					func(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
						log := log.FromContext(ctx).WithValues("request", req.String())
						log.Info("Reconciling ConfigMap")

						cl, err := mgr.GetCluster(ctx, req.ClusterName)
						if err != nil {
							return reconcile.Result{}, fmt.Errorf("failed to get cluster: %w", err)
						}

						cm := &corev1.ConfigMap{}
						if err := cl.GetClient().Get(ctx, req.NamespacedName, cm); err != nil {
							if apierrors.IsNotFound(err) {
								return reconcile.Result{}, nil
							}
							return reconcile.Result{}, fmt.Errorf("failed to get configmap: %w", err)
						}
						if cm.GetLabels()["test"] != "worker" {
							return reconcile.Result{}, nil
						}

						cm.Data = map[string]string{"status": "processed"}
						if err := cl.GetClient().Update(ctx, cm); err != nil {
							return reconcile.Result{}, fmt.Errorf("failed to update configmap: %w", err)
						}

						return ctrl.Result{}, nil
					},
				))
			Expect(err).NotTo(HaveOccurred())
		})

		By("Adding an index to the provider clusters", func() {
			err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.ConfigMap{}, "test", func(obj client.Object) []string {
				return []string{obj.GetLabels()["test"]}
			})
			Expect(err).NotTo(HaveOccurred())
		})

		By("Starting the provider, cluster, manager, and controller", func() {
			wg.Add(1)
			go func() {
				err := ignoreCanceled(mgr.Start(ctx))
				Expect(err).NotTo(HaveOccurred())
				wg.Done()
			}()
		})
	})

	BeforeAll(func() {
		runtime.Must(client.IgnoreAlreadyExists(workerCli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: workerClusterNamespace}})))
		runtime.Must(client.IgnoreAlreadyExists(workerCli.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: workerClusterNamespace,
				Name:      "test-cm",
				Labels:    map[string]string{"test": "worker"},
			},
		})))
	})

	It("lists the clusters loaded from Worker resources", func() {
		Eventually(provider.ListClusters, "10s").Should(HaveLen(1))
		Expect(provider.ListClusters()).To(ContainElement("testing/test-worker"))
	})

	It("runs the reconciler for existing objects", func(ctx context.Context) {
		Eventually(func() string {
			cm := &corev1.ConfigMap{}
			err := workerCli.Get(ctx, client.ObjectKey{Namespace: workerClusterNamespace, Name: "test-cm"}, cm)
			Expect(err).NotTo(HaveOccurred())
			return cm.Data["status"]
		}, "10s").Should(Equal("processed"))
	})

	It("runs the reconciler for new objects", func(ctx context.Context) {
		By("Creating a new configmap", func() {
			err := workerCli.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: workerClusterNamespace,
					Name:      "test-cm-2",
					Labels:    map[string]string{"test": "worker"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		Eventually(func() string {
			cm := &corev1.ConfigMap{}
			err := workerCli.Get(ctx, client.ObjectKey{Namespace: workerClusterNamespace, Name: "test-cm-2"}, cm)
			Expect(err).NotTo(HaveOccurred())
			return cm.Data["status"]
		}, "10s").Should(Equal("processed"))
	})

	It("runs the reconciler for updated objects", func(ctx context.Context) {
		updated := &corev1.ConfigMap{}
		By("Clearing the configmap data", func() {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if err := workerCli.Get(ctx, client.ObjectKey{Namespace: workerClusterNamespace, Name: "test-cm"}, updated); err != nil {
					return err
				}
				updated.Data = map[string]string{}
				return workerCli.Update(ctx, updated)
			})
			Expect(err).NotTo(HaveOccurred())
		})
		rv, err := strconv.ParseInt(updated.ResourceVersion, 10, 64)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int64 {
			cm := &corev1.ConfigMap{}
			err := workerCli.Get(ctx, client.ObjectKey{Namespace: workerClusterNamespace, Name: "test-cm"}, cm)
			Expect(err).NotTo(HaveOccurred())
			rv, err := strconv.ParseInt(cm.ResourceVersion, 10, 64)
			Expect(err).NotTo(HaveOccurred())
			return rv
		}, "10s").Should(BeNumerically(">=", rv))

		Eventually(func() string {
			cm := &corev1.ConfigMap{}
			err := workerCli.Get(ctx, client.ObjectKey{Namespace: workerClusterNamespace, Name: "test-cm"}, cm)
			Expect(err).NotTo(HaveOccurred())
			return cm.Data["status"]
		}, "10s").Should(Equal("processed"))
	})

	It("queries the cluster via a multi-cluster index", func() {
		worker, err := mgr.GetCluster(ctx, "testing/test-worker")
		Expect(err).NotTo(HaveOccurred())

		cms := &corev1.ConfigMapList{}
		err = worker.GetCache().List(ctx, cms, client.MatchingFields{"test": "worker"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cms.Items).To(HaveLen(2))
	})

	It("verifies worker status is set to Ready after successful job creation verification", func(ctx context.Context) {
		// Get the worker and check its status
		worker := &apev1.Worker{}
		err := localCli.Get(ctx, client.ObjectKey{Namespace: workerNamespace, Name: "test-worker"}, worker)
		Expect(err).NotTo(HaveOccurred())

		// Check that the Ready condition is set to True with reason "Ready"
		readyCondition := meta.FindStatusCondition(worker.Status.Conditions, apev1.WorkerConditionReady)
		Expect(readyCondition).NotTo(BeNil())
		Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCondition.Reason).To(Equal("Ready"))
		Expect(readyCondition.Message).To(ContainSubstring("ready and can create jobs"))
	})

	It("creates RBAC resources for testrun jobs in the worker cluster", func(ctx context.Context) {
		// Verify that the ClusterRole was created
		clusterRole := &rbacv1.ClusterRole{}
		err := workerCli.Get(ctx, client.ObjectKey{Name: testrunJobRoleName}, clusterRole)
		Expect(err).NotTo(HaveOccurred(), "ClusterRole should be created")

		// Verify the ClusterRole has the expected rules
		Expect(clusterRole.Rules).To(HaveLen(2))

		// Verify first rule (metrics.k8s.io pods)
		Expect(clusterRole.Rules[0].APIGroups).To(Equal([]string{"metrics.k8s.io"}))
		Expect(clusterRole.Rules[0].Resources).To(Equal([]string{"pods"}))
		Expect(clusterRole.Rules[0].Verbs).To(Equal([]string{"get", "list", "watch"}))

		// Verify second rule (core resources)
		Expect(clusterRole.Rules[1].APIGroups).To(Equal([]string{""}))
		Expect(clusterRole.Rules[1].Resources).To(ContainElements("nodes", "persistentvolumes"))
		Expect(clusterRole.Rules[1].Verbs).To(Equal([]string{"get", "list", "watch"}))

		// Verify the ClusterRole labels
		Expect(clusterRole.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "orderly-ape"))
		Expect(clusterRole.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "orderly-ape"))

		// Verify that the ClusterRoleBinding was created
		bindingName := workerClusterNamespace + "-testrun-job-role-binding"
		clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		err = workerCli.Get(ctx, client.ObjectKey{Name: bindingName}, clusterRoleBinding)
		Expect(err).NotTo(HaveOccurred(), "ClusterRoleBinding should be created")

		// Verify the ClusterRoleBinding references the correct ClusterRole
		Expect(clusterRoleBinding.RoleRef.Name).To(Equal(testrunJobRoleName))
		Expect(clusterRoleBinding.RoleRef.Kind).To(Equal("ClusterRole"))

		// Verify the ClusterRoleBinding subjects
		Expect(clusterRoleBinding.Subjects).To(HaveLen(1))
		Expect(clusterRoleBinding.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
		Expect(clusterRoleBinding.Subjects[0].Name).To(Equal("default"))
		Expect(clusterRoleBinding.Subjects[0].Namespace).To(Equal(workerClusterNamespace))

		// Verify the ClusterRoleBinding labels
		Expect(clusterRoleBinding.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "orderly-ape"))
		Expect(clusterRoleBinding.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "orderly-ape"))
	})

	It("removes a cluster from the provider when the Worker is deleted", func() {
		err := localCli.Delete(ctx, &apev1.Worker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-worker",
				Namespace: workerNamespace,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(provider.ListClusters, "10s").Should(BeEmpty())
	})

	AfterAll(func() {
		By("Stopping the provider, cluster, manager, and controller", func() {
			cancel()
			wg.Wait()
		})
	})
})

func ignoreCanceled(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func createKubeConfig(cfg *rest.Config) ([]byte, error) {
	name := "cluster"
	apiConfig := api.Config{
		Clusters: map[string]*api.Cluster{
			name: {
				Server:                   cfg.Host,
				CertificateAuthorityData: cfg.CAData,
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			name: {
				ClientCertificateData: cfg.CertData,
				ClientKeyData:         cfg.KeyData,
				Token:                 cfg.BearerToken,
			},
		},
		Contexts: map[string]*api.Context{
			name: {
				Cluster:  name,
				AuthInfo: name,
			},
		},
		CurrentContext: name,
	}
	kubeconfigData, err := clientcmd.Write(apiConfig)
	if err != nil {
		return nil, err
	}
	return kubeconfigData, nil
}

func createKubeconfigSecret(ctx context.Context, name string, cfg *rest.Config, namespace string, cl client.Client) error {
	kubeconfigData, err := createKubeConfig(cfg)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	secret.Data = map[string][]byte{
		"kubeconfig": kubeconfigData,
	}
	return cl.Create(ctx, secret)
}

// mockCluster is a mock implementation of cluster.Cluster for testing.
type mockCluster struct {
	cluster.Cluster
}

func (c *mockCluster) GetFieldIndexer() client.FieldIndexer {
	return &mockFieldIndexer{}
}

type mockFieldIndexer struct{}

func (f *mockFieldIndexer) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	// Simulate work to increase chance of race
	time.Sleep(time.Millisecond)
	return nil
}

var _ = Describe("Provider race condition", func() {
	It("should handle concurrent operations without issues", func() {
		p := New(Options{})

		// Pre-populate with some clusters to make the test meaningful
		numClusters := 20
		for i := 0; i < numClusters; i++ {
			clusterName := fmt.Sprintf("cluster-%d", i)
			p.clusters[clusterName] = activeCluster{
				Cluster: &mockCluster{},
				Cancel:  func() {},
			}
		}

		var wg sync.WaitGroup
		numGoroutines := 40
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(i int) {
				defer GinkgoRecover()
				defer wg.Done()

				// Mix of operations to stress the provider
				switch i % 4 {
				case 0:
					// Concurrently index a field. This will read the cluster list.
					err := p.IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName", func(rawObj client.Object) []string {
						return nil
					})
					Expect(err).NotTo(HaveOccurred())
				case 1:
					// Concurrently get a cluster.
					_, err := p.Get(context.Background(), "cluster-1")
					Expect(err).To(Or(BeNil(), MatchError(mcluster.ErrClusterNotFound)))
				case 2:
					// Concurrently list clusters.
					p.ListClusters()
				case 3:
					// Concurrently delete a cluster. This will modify the cluster map.
					clusterToRemove := fmt.Sprintf("cluster-%d", i/4)
					p.removeCluster(clusterToRemove)
				}
			}(i)
		}

		wg.Wait()
	})
})
