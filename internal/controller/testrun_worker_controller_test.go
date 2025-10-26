// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/providers/worker"
	kutil "github.com/ReviewSignal/orderly-ape/internal/util/test/kubernetes"
)

const (
	localNamespace         = "orderly-ape"
	workerClusterNamespace = "worker"
	testTimeout            = 10 * time.Second
)

var _ = Describe("TestRunWorker Controller", Ordered, func() {
	var (
		workerEnv        *envtest.Environment
		workerCfg        *rest.Config
		workerCli        client.Client
		mgr              mcmanager.Manager
		provider         *worker.Provider
		reconciler       *TestRunWorkerReconciler
		ctx              context.Context
		cancel           context.CancelFunc
		wg               sync.WaitGroup
		testRunName      = "test-run-worker"
		workerName       = "test-worker"
		influxDBName     = "test-influxdb"
		testScenarioName = "test-scenario"
	)

	BeforeAll(func() {
		var err error
		ctx, cancel = context.WithCancel(context.Background())

		// Setup worker test environment
		By("bootstrapping worker test environment")
		workerEnv = &envtest.Environment{}

		// Retrieve the first found binary directory to allow running tests from IDEs
		if getFirstFoundEnvTestBinaryDir() != "" {
			workerEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
		}

		workerCfg, err = workerEnv.Start()
		Expect(err).NotTo(HaveOccurred())

		workerCli, err = client.New(workerCfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		// Create namespaces
		By("creating namespaces")
		err = k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: localNamespace},
		})
		Expect(err).NotTo(HaveOccurred())

		// Worker cluster's default namespace already exists, so ignore AlreadyExists error
		err = workerCli.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: workerClusterNamespace},
		})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		// Create kubeconfig secret for worker
		By("creating kubeconfig secret")
		kubeconfigData, err := createKubeConfig(workerCfg)
		Expect(err).NotTo(HaveOccurred())

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker-secret",
				Namespace: localNamespace,
			},
			Data: map[string][]byte{
				"kubeconfig": kubeconfigData,
			},
		}
		err = k8sClient.Create(ctx, secret)
		Expect(err).NotTo(HaveOccurred())

		// Create Worker resource
		By("creating Worker resource")
		workerResource := &apev1.Worker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workerName,
				Namespace: localNamespace,
			},
			Spec: apev1.WorkerSpec{
				KubeconfigSecret: &corev1.SecretReference{
					Name:      "worker-secret",
					Namespace: localNamespace,
				},
				Namespace: workerClusterNamespace,
			},
		}
		err = k8sClient.Create(ctx, workerResource)
		Expect(err).NotTo(HaveOccurred())

		// Setup provider
		By("setting up worker provider")
		provider = worker.New(worker.Options{
			Namespace:              localNamespace,
			DefaultWorkerNamespace: workerClusterNamespace,
			ClusterOptions: []cluster.Option{
				func(clusterOptions *cluster.Options) {
					clusterOptions.Scheme = scheme.Scheme
				},
			},
		})

		// Setup multicluster manager
		By("setting up multicluster manager")
		mgr, err = mcmanager.New(cfg, provider, mcmanager.Options{
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		err = provider.SetupWithManager(ctx, mgr)
		Expect(err).NotTo(HaveOccurred())

		// Setup TestRunWorker controller
		By("setting up TestRunWorker controller")
		reconciler = &TestRunWorkerReconciler{
			Client:         mgr.GetLocalManager().GetClient(),
			Scheme:         mgr.GetLocalManager().GetScheme(),
			LocalNamespace: localNamespace,
		}

		err = reconciler.SetupWithManager(mgr)
		Expect(err).NotTo(HaveOccurred())

		// Start manager
		By("starting manager")
		wg.Add(1)
		go func() {
			defer GinkgoRecover()
			err := ignoreCanceled(mgr.Start(ctx))
			Expect(err).NotTo(HaveOccurred())
			wg.Done()
		}()

		// Wait for worker cluster to be engaged
		By("waiting for worker cluster to be engaged")
		Eventually(provider.ListClusters, testTimeout).Should(ContainElement(localNamespace + "/" + workerName))
	})

	AfterAll(func() {
		By("tearing down test environments")
		cancel()
		wg.Wait()

		if workerEnv != nil {
			Expect(workerEnv.Stop()).To(Succeed())
		}
	})

	BeforeEach(func() {
		By("creating InfluxDB resource")
		influxDB := &apev1.InfluxDB{
			ObjectMeta: metav1.ObjectMeta{
				Name:      influxDBName,
				Namespace: localNamespace,
			},
			Spec: apev1.InfluxDBSpec{
				Address: apev1.SourcedValue{
					Value: "http://localhost:8086",
				},
				Token: apev1.SourcedValue{
					Value: "test-token",
				},
				Organization: apev1.SourcedValue{
					Value: "test-org",
				},
			},
		}
		Expect(k8sClient.Create(ctx, influxDB)).To(Succeed())

		influxDB.Status.OrganizationID = "test-org-id"
		Expect(k8sClient.Status().Update(ctx, influxDB)).To(Succeed())

		By("creating TestScenario resource")
		testScenario := &apev1.TestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testScenarioName,
				Namespace: localNamespace,
			},
			Spec: apev1.TestScenarioSpec{
				TestSource: apev1.TestSource{
					GitRepo: &apev1.TestGitRepoSource{
						Repository: "https://github.com/example/test-repo",
						Revision:   "main",
						Path:       "loadtest.js",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, testScenario)).To(Succeed())

		By("creating InfluxDB credentials secret")
		credentialsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testRunName + "-influxdb-creds",
				Namespace: localNamespace,
			},
			StringData: map[string]string{
				"INFLUXDB_HOST":   "http://localhost:8086",
				"INFLUXDB_ORG":    "test-org",
				"INFLUXDB_BUCKET": "test-bucket",
				"INFLUXDB_TOKEN":  "test-token",
			},
		}
		Expect(k8sClient.Create(ctx, credentialsSecret)).To(Succeed())
	})

	AfterEach(func(ctx context.Context) {
		By("cleaning up TestRun")
		testRun := &apev1.TestRun{ObjectMeta: metav1.ObjectMeta{Name: testRunName, Namespace: localNamespace}}
		Expect(kutil.Cleanup(ctx, k8sClient, testRun)).To(Succeed())

		By("cleaning up Job in worker cluster")
		job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: testRunName, Namespace: workerClusterNamespace}}
		Expect(kutil.Cleanup(ctx, workerCli, job)).To(Succeed())

		By("cleaning up secrets")
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testRunName + "-influxdb-creds", Namespace: localNamespace}}
		Expect(kutil.Cleanup(ctx, k8sClient, secret)).To(Succeed())

		By("cleaning up TestScenario")
		scenario := &apev1.TestScenario{ObjectMeta: metav1.ObjectMeta{Name: testScenarioName, Namespace: localNamespace}}
		Expect(kutil.Cleanup(ctx, k8sClient, scenario)).To(Succeed())

		By("cleaning up InfluxDB")
		influxDB := &apev1.InfluxDB{ObjectMeta: metav1.ObjectMeta{Name: influxDBName, Namespace: localNamespace}}
		Expect(kutil.Cleanup(ctx, k8sClient, influxDB)).To(Succeed())
	})

	Context("When reconciling a TestRun", func() {
		It("should transition from Pending to Queued when Job is created", func() {
			By("creating TestRun with Pending worker status")
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: localNamespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					State:  apev1.TestRunStateActive,
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: localNamespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: localNamespace,
					},
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      workerName,
								Namespace: localNamespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			By("updating TestRun status")
			Eventually(func() error {
				// Refetch to avoid conflicts with controller updates
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, testRun); err != nil {
					return err
				}
				testRun.Status.InfluxDBSecret = &apev1.ObjectReference{
					Name:      testRunName + "-influxdb-creds",
					Namespace: localNamespace,
				}
				testRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
					{
						Name:             workerName,
						Phase:            apev1.TestRunPhasePending,
						AssignedSegments: []string{"0:1"},
					},
				}
				testRun.Status.ScenarioSpec = &apev1.TestRunScenarioSpec{
					TestSource: apev1.TestSource{
						GitRepo: &apev1.TestGitRepoSource{
							Repository: "https://github.com/example/test-repo",
							Revision:   "main",
							Path:       "loadtest.js",
						},
					},
				}
				return k8sClient.Status().Update(ctx, testRun)
			}, testTimeout).Should(Succeed())

			By("waiting for Job to be created in worker cluster")
			Eventually(func() error {
				job := &batchv1.Job{}
				return workerCli.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: workerClusterNamespace,
				}, job)
			}, testTimeout).Should(Succeed())

			By("verifying worker phase transitioned to Queued")
			Eventually(func() apev1.TestRunPhase {
				updatedTestRun := &apev1.TestRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, updatedTestRun)
				if err != nil {
					return apev1.TestRunPhaseUnknown
				}
				if len(updatedTestRun.Status.WorkerStatuses) == 0 {
					return apev1.TestRunPhaseUnknown
				}
				return updatedTestRun.Status.WorkerStatuses[0].Phase
			}, testTimeout).Should(Equal(apev1.TestRunPhaseQueued))
		})

		It("should handle TestRun cancellation", func() {
			By("creating TestRun with Queued worker status")
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: localNamespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					State:  apev1.TestRunStateActive,
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: localNamespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: localNamespace,
					},
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      workerName,
								Namespace: localNamespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			By("updating TestRun status")
			Eventually(func() error {
				// Refetch to avoid conflicts with controller updates
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, testRun); err != nil {
					return err
				}
				testRun.Status.InfluxDBSecret = &apev1.ObjectReference{
					Name:      testRunName + "-influxdb-creds",
					Namespace: localNamespace,
				}
				testRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
					{
						Name:             workerName,
						Phase:            apev1.TestRunPhasePending,
						AssignedSegments: []string{"0:1"},
					},
				}
				testRun.Status.ScenarioSpec = &apev1.TestRunScenarioSpec{
					TestSource: apev1.TestSource{
						GitRepo: &apev1.TestGitRepoSource{
							Repository: "https://github.com/example/test-repo",
							Revision:   "main",
							Path:       "loadtest.js",
						},
					},
				}
				return k8sClient.Status().Update(ctx, testRun)
			}, testTimeout).Should(Succeed())

			By("waiting for Job to be created")
			Eventually(func() error {
				job := &batchv1.Job{}
				return workerCli.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: workerClusterNamespace,
				}, job)
			}, testTimeout).Should(Succeed())

			By("canceling the TestRun")
			updatedTestRun := &apev1.TestRun{}
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, updatedTestRun)
				if err != nil {
					return err
				}
				updatedTestRun.Spec.State = apev1.TestRunStateCanceled
				return k8sClient.Update(ctx, updatedTestRun)
			}, testTimeout).Should(Succeed())

			By("verifying Job is suspended")
			Eventually(func() bool {
				job := &batchv1.Job{}
				err := workerCli.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: workerClusterNamespace,
				}, job)
				if err != nil {
					return false
				}
				return job.Spec.Suspend != nil && *job.Spec.Suspend
			}, testTimeout).Should(BeTrue())

			By("verifying worker phase is Canceled")
			Eventually(func() apev1.TestRunPhase {
				updatedTestRun := &apev1.TestRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, updatedTestRun)
				if err != nil {
					return apev1.TestRunPhaseUnknown
				}
				if len(updatedTestRun.Status.WorkerStatuses) == 0 {
					return apev1.TestRunPhaseUnknown
				}
				return updatedTestRun.Status.WorkerStatuses[0].Phase
			}, testTimeout).Should(Equal(apev1.TestRunPhaseCanceled))

			By("verifying Canceled condition is set")
			Eventually(func() bool {
				updatedTestRun := &apev1.TestRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, updatedTestRun)
				if err != nil {
					return false
				}
				if len(updatedTestRun.Status.WorkerStatuses) == 0 {
					return false
				}
				cond := meta.FindStatusCondition(updatedTestRun.Status.WorkerStatuses[0].Conditions,
					apev1.TestRunWorkerConditionReady)
				return cond != nil && cond.Status == metav1.ConditionFalse &&
					cond.Reason == apev1.TestRunWorkerReasonCanceled
			}, testTimeout).Should(BeTrue())
		})

		It("should handle TestRun deletion", func() {
			By("creating TestRun")
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: localNamespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					State:  apev1.TestRunStateActive,
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: localNamespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: localNamespace,
					},
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      workerName,
								Namespace: localNamespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			By("updating TestRun status")
			Eventually(func() error {
				// Refetch to avoid conflicts with controller updates
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, testRun); err != nil {
					return err
				}
				testRun.Status.InfluxDBSecret = &apev1.ObjectReference{
					Name:      testRunName + "-influxdb-creds",
					Namespace: localNamespace,
				}
				testRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
					{
						Name:             workerName,
						Phase:            apev1.TestRunPhasePending,
						AssignedSegments: []string{"0:1"},
					},
				}
				testRun.Status.ScenarioSpec = &apev1.TestRunScenarioSpec{
					TestSource: apev1.TestSource{
						GitRepo: &apev1.TestGitRepoSource{
							Repository: "https://github.com/example/test-repo",
							Revision:   "main",
							Path:       "loadtest.js",
						},
					},
				}
				return k8sClient.Status().Update(ctx, testRun)
			}, testTimeout).Should(Succeed())

			By("waiting for Job to be created")
			Eventually(func() error {
				job := &batchv1.Job{}
				return workerCli.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: workerClusterNamespace,
				}, job)
			}, testTimeout).Should(Succeed())

			By("deleting the TestRun")
			Expect(k8sClient.Delete(ctx, testRun)).To(Succeed())

			By("verifying Job is deleted from worker cluster")
			Eventually(func() bool {
				job := &batchv1.Job{}
				err := workerCli.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: workerClusterNamespace,
				}, job)
				return apierrors.IsNotFound(err)
			}, testTimeout).Should(BeTrue())
		})

		It("should mark worker as Failed when Job is not found for Pending phase", func() {
			By("creating TestRun with Pending status but no Job")
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: localNamespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					State:  apev1.TestRunStateActive,
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: localNamespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: localNamespace,
					},
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      workerName,
								Namespace: localNamespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			By("updating TestRun status with Pending phase")
			Eventually(func() error {
				// Refetch to avoid conflicts with controller updates
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, testRun); err != nil {
					return err
				}
				testRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
					{
						Name:             workerName,
						Phase:            apev1.TestRunPhasePending,
						AssignedSegments: []string{"0:1"},
					},
				}
				return k8sClient.Status().Update(ctx, testRun)
			}, testTimeout).Should(Succeed())

			By("verifying worker phase transitions to Failed")
			Eventually(func() apev1.TestRunPhase {
				updatedTestRun := &apev1.TestRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, updatedTestRun)
				if err != nil {
					return apev1.TestRunPhaseUnknown
				}
				if len(updatedTestRun.Status.WorkerStatuses) == 0 {
					return apev1.TestRunPhaseUnknown
				}
				return updatedTestRun.Status.WorkerStatuses[0].Phase
			}, testTimeout).Should(Equal(apev1.TestRunPhaseFailed))

			By("verifying Failed condition is set")
			Eventually(func() bool {
				updatedTestRun := &apev1.TestRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRunName,
					Namespace: localNamespace,
				}, updatedTestRun)
				if err != nil {
					return false
				}
				if len(updatedTestRun.Status.WorkerStatuses) == 0 {
					return false
				}
				cond := meta.FindStatusCondition(updatedTestRun.Status.WorkerStatuses[0].Conditions,
					apev1.TestRunWorkerConditionFailed)
				return cond != nil && cond.Status == metav1.ConditionTrue &&
					cond.Reason == apev1.TestRunWorkerReasonJobCreateFailed
			}, testTimeout).Should(BeTrue())
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
	return clientcmd.Write(apiConfig)
}

var _ = Describe("TestRunWorker VictoriaLogs Integration", func() {
	Context("When syncing Telegraf configuration with VictoriaLogs", func() {
		It("should include VictoriaLogs configuration in Telegraf config", func() {
			// This test validates that the Telegraf template includes VictoriaLogs config
			// when VictoriaLogs is enabled

			var buf bytes.Buffer
			err := telegrafConfigTemplate.Execute(&buf, map[string]any{
				"Interval":      10 * time.Second,
				"FlushInterval": 10 * time.Second,
				"FlushJitter":   5 * time.Second,
				"TestID":        "test-run",
				"Worker":        "test-worker",
				"InfluxDB": map[string]string{
					"Host":   "http://influxdb:8086",
					"Token":  "test-token",
					"Org":    "test-org",
					"Bucket": "test-bucket",
				},
				"VictoriaLogs": map[string]any{
					"InsertURL": "http://victorialogs:9428/insert",
					"TestID":    "test-run",
					"Worker":    "test-worker",
					"ProjectID": "12345",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			config := buf.String()

			// Verify VictoriaLogs output configuration is present
			Expect(config).To(ContainSubstring("[[outputs.http]]"))
			Expect(config).To(ContainSubstring("/jsonline"))
			Expect(config).To(ContainSubstring("test-run"))
			Expect(config).To(ContainSubstring("test-worker"))

			// Verify log input configuration is present
			Expect(config).To(ContainSubstring("[[inputs.tail]]"))
			Expect(config).To(ContainSubstring("/var/log/containers/*.log"))
		})

		It("should not include token when not using proxied insert", func() {
			// Generate config without token
			var buf bytes.Buffer
			err := telegrafConfigTemplate.Execute(&buf, map[string]any{
				"Interval":      10 * time.Second,
				"FlushInterval": 10 * time.Second,
				"FlushJitter":   5 * time.Second,
				"TestID":        "test-run",
				"Worker":        "test-worker",
				"InfluxDB": map[string]string{
					"Host":   "http://influxdb:8086",
					"Token":  "test-token",
					"Org":    "test-org",
					"Bucket": "test-bucket",
				},
				"VictoriaLogs": map[string]any{
					"InsertURL": "http://victorialogs:9428/insert",
					"TestID":    "test-run",
					"Worker":    "test-worker",
					"ProjectID": "12345",
					// No Token field
				},
			})

			Expect(err).NotTo(HaveOccurred())
			config := buf.String()

			// Verify no Authorization header is present
			Expect(config).NotTo(ContainSubstring("Authorization = \"Bearer"))
		})

		It("should include Authorization header when token is provided", func() {
			var buf bytes.Buffer
			err := telegrafConfigTemplate.Execute(&buf, map[string]any{
				"Interval":      10 * time.Second,
				"FlushInterval": 10 * time.Second,
				"FlushJitter":   5 * time.Second,
				"TestID":        "test-run",
				"Worker":        "test-worker",
				"InfluxDB": map[string]string{
					"Host":   "http://influxdb:8086",
					"Token":  "test-token",
					"Org":    "test-org",
					"Bucket": "test-bucket",
				},
				"VictoriaLogs": map[string]any{
					"InsertURL": "http://victorialogs:9428/insert",
					"TestID":    "test-run",
					"Worker":    "test-worker",
					"ProjectID": "12345",
					"Token":     "test-jwt-token",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			config := buf.String()

			// Verify Authorization header is present with token
			Expect(config).To(ContainSubstring("Authorization = \"Bearer test-jwt-token\""))
		})

		It("should include Kubernetes resource monitoring in Telegraf config", func() {
			// This test validates that the Telegraf template includes kubernetes inputs
			// for pod resource monitoring

			var buf bytes.Buffer
			err := telegrafConfigTemplate.Execute(&buf, map[string]any{
				"Interval":      10 * time.Second,
				"FlushInterval": 10 * time.Second,
				"FlushJitter":   5 * time.Second,
				"TestID":        "test-run",
				"Worker":        "test-worker",
				"InfluxDB": map[string]string{
					"Host":   "http://influxdb:8086",
					"Token":  "test-token",
					"Org":    "test-org",
					"Bucket": "test-bucket",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			config := buf.String()

			// Verify Kubernetes input configuration is present
			Expect(config).To(ContainSubstring("[[inputs.kubernetes]]"))
			Expect(config).To(ContainSubstring("https://${HOST_IP}:10250"))
			Expect(config).To(ContainSubstring("testid = \"test-run\""))
			Expect(config).To(ContainSubstring("location = \"test-worker\""))
		})
	})
})
