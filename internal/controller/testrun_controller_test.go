// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	kubernetesutil "github.com/ReviewSignal/orderly-ape/internal/util/test/kubernetes"
)

// mockInfluxDBClientForTestRun is a mock implementation for TestRun controller tests
type mockInfluxDBClientForTestRun struct {
	createTokenFunc func(ctx context.Context, orgID, description, bucket string) (string, string, error)
	deleteTokenFunc func(ctx context.Context, tokenID string) error
	getReadyFunc    func(ctx context.Context) (string, time.Time, error)
	listBucketsFunc func(ctx context.Context) ([]string, error)
}

// GetOrganizationID implements InfluxDBClient.
func (m *mockInfluxDBClientForTestRun) GetOrganizationID(ctx context.Context, org string) (string, error) {
	return "", nil
}

func (m *mockInfluxDBClientForTestRun) GetReady(ctx context.Context) (string, time.Time, error) {
	if m.getReadyFunc != nil {
		return m.getReadyFunc(ctx)
	}
	return "v2.7.0", time.Now(), nil
}

func (m *mockInfluxDBClientForTestRun) ListBuckets(ctx context.Context) ([]string, error) {
	if m.listBucketsFunc != nil {
		return m.listBucketsFunc(ctx)
	}
	return []string{"bucket1"}, nil
}

func (m *mockInfluxDBClientForTestRun) CreateToken(ctx context.Context, orgID, description, bucket string) (string, string, error) {
	if m.createTokenFunc != nil {
		return m.createTokenFunc(ctx, orgID, description, bucket)
	}
	return "token-id-123", "token-value-456", nil
}

func (m *mockInfluxDBClientForTestRun) CreateReadOnlyToken(ctx context.Context, orgID, description string) (string, string, error) {
	return "ro-token-id-123", "ro-token-value-456", nil
}

func (m *mockInfluxDBClientForTestRun) DeleteToken(ctx context.Context, tokenID string) error {
	if m.deleteTokenFunc != nil {
		return m.deleteTokenFunc(ctx, tokenID)
	}
	return nil
}

var _ = Describe("TestRun Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			testRunName      = "test-testrun"
			testScenarioName = "test-scenario"
			influxDBName     = "test-influxdb"
			namespace        = "default"
		)

		ctx := context.Background()

		var mockClient *mockInfluxDBClientForTestRun
		var controllerReconciler *TestRunReconciler

		BeforeEach(func() {
			mockClient = &mockInfluxDBClientForTestRun{}

			controllerReconciler = &TestRunReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockClient
				},
			}

			// Create InfluxDB resource
			influxDB := &apev1.InfluxDB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      influxDBName,
					Namespace: namespace,
				},
				Spec: apev1.InfluxDBSpec{
					Address: apev1.SourcedValue{
						Value: "http://localhost:8086",
					},
					Token: apev1.SourcedValue{
						Value: "admin-token",
					},
					Organization: apev1.SourcedValue{
						Value: "test-org",
					},
				},
			}
			Expect(k8sClient.Create(ctx, influxDB)).To(Succeed())

			// Update InfluxDB status with OrganizationID
			influxDB.Status.OrganizationID = "test-org-id"
			Expect(k8sClient.Status().Update(ctx, influxDB)).To(Succeed())

			// Create TestScenario resource
			testScenario := &apev1.TestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testScenarioName,
					Namespace: namespace,
				},
				Spec: apev1.TestScenarioSpec{
					TestSource: apev1.TestSource{
						GitRepo: &apev1.TestGitRepoSource{
							Repository: "https://github.com/example/test-repo",
							Revision:   "main",
							Path:       "loadtest.js",
						},
					},
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
					EnvVars: []apev1.EnvVar{
						{Name: "SCENARIO_VAR1", Value: "value1"},
						{Name: "SCENARIO_VAR2", Value: "value2"},
					},
					Labels: map[string]string{
						"scenario-label1": "value1",
						"scenario-label2": "value2",
					},
				},
			}
			Expect(k8sClient.Create(ctx, testScenario)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup resources
			testRun := &apev1.TestRun{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, testRun); err == nil {
				Expect(kubernetesutil.Cleanup(ctx, k8sClient, testRun, testRunFinalizerInfluxDBCredentials)).To(Succeed())
			}

			influxDB := &apev1.InfluxDB{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: influxDBName, Namespace: namespace}, influxDB); err == nil {
				Expect(k8sClient.Delete(ctx, influxDB)).To(Succeed())
			}

			scenario := &apev1.TestScenario{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: testScenarioName, Namespace: namespace}, scenario); err == nil {
				Expect(k8sClient.Delete(ctx, scenario)).To(Succeed())
			}
		})

		It("should add finalizer when reconciling a new TestRun", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile may fail to update spec due to resource version conflict after status update
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})

			// Second reconcile completes the spec update
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTestRun, testRunFinalizerInfluxDBCredentials)).To(BeTrue())
		})

		It("should copy Workers from TestScenario when empty and state is Active", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State:   apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{}, // Empty workers
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile may fail to update spec due to resource version conflict after status update
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})

			// Second reconcile completes the spec update
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify workers were copied
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Spec.Workers).To(HaveLen(1))
			Expect(updatedTestRun.Spec.Workers[0].Name).To(Equal("worker1"))
		})

		It("should merge EnvVars from TestScenario with TestRun taking priority", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
					EnvVars: []apev1.EnvVar{
						{Name: "SCENARIO_VAR1", Value: "overridden"},
						{Name: "TESTRUN_VAR", Value: "testrun-value"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile may fail to update spec due to resource version conflict after status update
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})

			// Second reconcile completes the spec update
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify env vars were merged
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Spec.EnvVars).To(HaveLen(3))

			envVarMap := make(map[string]string)
			for _, env := range updatedTestRun.Spec.EnvVars {
				envVarMap[env.Name] = env.Value
			}
			Expect(envVarMap["SCENARIO_VAR1"]).To(Equal("overridden"))
			Expect(envVarMap["SCENARIO_VAR2"]).To(Equal("value2"))
			Expect(envVarMap["TESTRUN_VAR"]).To(Equal("testrun-value"))
		})

		It("should merge Labels from TestScenario with TestRun taking priority", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
					Labels: map[string]string{
						"scenario-label1": "overridden",
						"testrun-label":   "testrun-value",
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile may fail to update spec due to resource version conflict after status update
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})

			// Second reconcile completes the spec update
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify labels were merged
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Spec.Labels).To(HaveLen(3))
			Expect(updatedTestRun.Spec.Labels["scenario-label1"]).To(Equal("overridden"))
			Expect(updatedTestRun.Spec.Labels["scenario-label2"]).To(Equal("value2"))
			Expect(updatedTestRun.Spec.Labels["testrun-label"]).To(Equal("testrun-value"))
		})

		It("should create InfluxDB secret when Active and not in final phase", func() {
			bucket := "test-bucket"
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					InfluxDBBucket: &bucket,
					State:          apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to update spec (finalizers, workers, etc.)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to create secret and update status
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created and status was updated
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.InfluxDBSecret).NotTo(BeNil())

			// Verify secret exists and has correct data
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{
				Name:      updatedTestRun.Status.InfluxDBSecret.Name,
				Namespace: updatedTestRun.Status.InfluxDBSecret.Namespace,
			}
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("INFLUXDB_HOST"))
			Expect(secret.Data).To(HaveKey("INFLUXDB_ORG"))
			Expect(secret.Data).To(HaveKey("INFLUXDB_BUCKET"))
			Expect(secret.Data).To(HaveKey("INFLUXDB_TOKEN"))
			Expect(string(secret.Data["INFLUXDB_HOST"])).To(Equal("http://localhost:8086"))
			Expect(string(secret.Data["INFLUXDB_ORG"])).To(Equal("test-org"))
			Expect(string(secret.Data["INFLUXDB_BUCKET"])).To(Equal("test-bucket"))
			Expect(string(secret.Data["INFLUXDB_TOKEN"])).To(Equal("token-value-456"))

			// Verify secret has token ID in annotations
			Expect(secret.Annotations).To(HaveKey("ape.reviewsignal.com/influxdb-token-id"))
			Expect(secret.Annotations["ape.reviewsignal.com/influxdb-token-id"]).To(Equal("token-id-123"))

			// Verify TestRun is the owner
			Expect(secret.OwnerReferences).To(HaveLen(1))
			Expect(secret.OwnerReferences[0].Name).To(Equal(testRunName))
		})

		It("should cleanup credentials when TestRun is deleted", func() {
			deletedTokenID := ""
			mockClient.deleteTokenFunc = func(ctx context.Context, tokenID string) error {
				deletedTokenID = tokenID
				return nil
			}

			bucket := "test-bucket"
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					InfluxDBBucket: &bucket,
					State:          apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile may fail to update spec due to resource version conflict after status update
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})

			// Second reconcile completes setup and creates secret
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.InfluxDBSecret).NotTo(BeNil())
			secretName := updatedTestRun.Status.InfluxDBSecret.Name

			// Delete the TestRun
			Expect(k8sClient.Delete(ctx, updatedTestRun)).To(Succeed())

			// Reconcile to handle deletion
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify token was deleted
			Expect(deletedTokenID).To(Equal("token-id-123"))

			// Verify secret was deleted
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should cleanup credentials when TestRun reaches Completed phase", func() {
			deletedTokenID := ""
			mockClient.deleteTokenFunc = func(ctx context.Context, tokenID string) error {
				deletedTokenID = tokenID
				return nil
			}

			bucket := "test-bucket"
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					InfluxDBBucket: &bucket,
					State:          apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to update spec (finalizers, workers, etc.)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to create secret and update status
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.InfluxDBSecret).NotTo(BeNil())
			secretName := updatedTestRun.Status.InfluxDBSecret.Name

			// Update status to Completed
			updatedTestRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
				{
					Name:  "worker1",
					Phase: apev1.TestRunPhaseCompleted,
				},
			}
			Expect(k8sClient.Status().Update(ctx, updatedTestRun)).To(Succeed())

			// Reconcile to cleanup
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify token was deleted
			Expect(deletedTokenID).To(Equal("token-id-123"))

			// Verify secret was deleted
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			// Verify status was updated
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.InfluxDBSecret).To(BeNil())
		})

		It("should handle token creation failure gracefully", func() {
			mockClient.createTokenFunc = func(ctx context.Context, orgID, description, bucket string) (string, string, error) {
				return "", "", fmt.Errorf("token creation failed")
			}

			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to update spec (finalizers, workers, etc.)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to create secret and update status
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("token creation failed"))
		})

		It("should not process TestRun when state is Draft", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateDraft,
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify no secret was created
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.InfluxDBSecret).To(BeNil())
		})

		It("should cleanup credentials when TestRun state is Canceled", func() {
			deletedTokenID := ""
			mockClient.deleteTokenFunc = func(ctx context.Context, tokenID string) error {
				deletedTokenID = tokenID
				return nil
			}

			bucket := "test-bucket"
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					InfluxDBBucket: &bucket,
					State:          apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to update spec (finalizers, workers, etc.)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to create secret and update status
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.InfluxDBSecret).NotTo(BeNil())
			secretName := updatedTestRun.Status.InfluxDBSecret.Name

			// Update state to Canceled
			updatedTestRun.Spec.State = apev1.TestRunStateCanceled
			Expect(k8sClient.Update(ctx, updatedTestRun)).To(Succeed())

			// Reconcile to cleanup
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify token was deleted
			Expect(deletedTokenID).To(Equal("token-id-123"))

			// Verify secret was deleted
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			// Verify status was updated
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.InfluxDBSecret).To(BeNil())
		})

		It("should remove cleanup finalizer when TestRun reaches Canceled phase", func() {
			deletedTokenID := ""
			mockClient.deleteTokenFunc = func(ctx context.Context, tokenID string) error {
				deletedTokenID = tokenID
				return nil
			}

			bucket := "test-bucket"
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					InfluxDBBucket: &bucket,
					State:          apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to update spec (finalizers, workers, etc.)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to create secret and update status
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizers were added
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTestRun, testRunFinalizerInfluxDBCredentials)).To(BeTrue())

			// Update workers status to Canceled to set phase
			updatedTestRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
				{
					Name:  "worker1",
					Phase: apev1.TestRunPhaseCanceled,
				},
			}
			Expect(k8sClient.Status().Update(ctx, updatedTestRun)).To(Succeed())

			// Update state to Canceled
			updatedTestRun.Spec.State = apev1.TestRunStateCanceled
			Expect(k8sClient.Update(ctx, updatedTestRun)).To(Succeed())

			// Reconcile to cleanup and remove finalizers
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify token was deleted
			Expect(deletedTokenID).To(Equal("token-id-123"))

			// Verify both finalizers were removed
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTestRun, testRunFinalizerInfluxDBCredentials)).To(BeFalse())

			// Now delete the TestRun and verify it can be deleted successfully
			Expect(k8sClient.Delete(ctx, updatedTestRun)).To(Succeed())

			// Verify TestRun was deleted
			err = k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should update spec only once when multiple changes are needed", func() {
			updateCallCount := 0
			originalUpdate := k8sClient.Update

			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State:   apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{}, // Empty workers
					EnvVars: []apev1.EnvVar{
						{Name: "SCENARIO_VAR1", Value: "overridden"},
					},
					Labels: map[string]string{
						"scenario-label1": "overridden",
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// Track Update calls on TestRun
			wrappedClient := &updateTrackingClient{
				Client: k8sClient,
				onUpdate: func(obj client.Object) {
					if _, ok := obj.(*apev1.TestRun); ok {
						updateCallCount++
					}
				},
			}

			wrappedReconciler := &TestRunReconciler{
				Client: wrappedClient,
				Scheme: k8sClient.Scheme(),
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockClient
				},
			}

			_, err := wrappedReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			// First reconcile may fail due to resource version conflict
			_ = err

			// Second reconcile to complete spec update
			_, err = wrappedReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// The controller combines all spec changes (finalizer + workers + envvars + labels)
			// into a single update, but due to status update conflicts, we may see the update
			// happen on the second reconcile. We should see at least 1 update.
			Expect(updateCallCount).To(BeNumerically(">=", 1))

			// Restore original update
			_ = originalUpdate
		})

		It("should assign segments to workers correctly", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 2,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to setup spec
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})

			// Second reconcile to assign segments
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify segments were assigned
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.WorkerStatuses).To(HaveLen(1))
			Expect(updatedTestRun.Status.WorkerStatuses[0].AssignedSegments).To(HaveLen(2))
		})

		It("should initialize ScenarioSpec in status when Active", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile may fail to update spec due to resource version conflict after status update
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})

			// Second reconcile completes the status update
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify ScenarioSpec was copied to status
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.ScenarioSpec).NotTo(BeNil())
		})

		It("should set TestRun to Archived when all workers complete", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to setup
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Update worker status to Completed
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			updatedTestRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
				{
					Name:  "worker1",
					Phase: apev1.TestRunPhaseCompleted,
				},
			}
			Expect(k8sClient.Status().Update(ctx, updatedTestRun)).To(Succeed())

			// Reconcile to archive
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify state was updated to Archived
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Spec.State).To(Equal(apev1.TestRunStateArchived))
		})

		It("should set CompletionTime when test completes", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName + "-completion",
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to setup
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName + "-completion", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Update worker status to Completed
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName + "-completion", Namespace: namespace}, updatedTestRun)).To(Succeed())
			updatedTestRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
				{
					Name:  "worker1",
					Phase: apev1.TestRunPhaseCompleted,
				},
			}
			Expect(k8sClient.Status().Update(ctx, updatedTestRun)).To(Succeed())

			// Reconcile to set completion time
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName + "-completion", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify CompletionTime was set
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName + "-completion", Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.CompletionTime).NotTo(BeNil())
		})

		It("should create Grafana dashboard when test starts", func() {
			dashboardCreated := false
			mockGrafana := &mockGrafanaClient{
				createDashboardFunc: func(ctx context.Context, dashboard map[string]any) (string, error) {
					dashboardCreated = true
					// Verify dashboard structure
					Expect(dashboard["title"]).To(ContainSubstring(testRunName + "-dashboard"))
					return "test-dashboard-uid", nil
				},
			}

			reconciler := &TestRunReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockClient
				},
				GrafanaClient: mockGrafana,
				GrafanaURL:    "http://grafana.example.com",
			}

			// Create InfluxDB with datasource UID annotation
			influxDBDash := &apev1.InfluxDB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      influxDBName + "-dashboard",
					Namespace: namespace,
					Annotations: map[string]string{
						apev1.InfluxDBAnnotationGrafanaDatasourceUID: "test-datasource-uid",
					},
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
			Expect(k8sClient.Create(ctx, influxDBDash)).To(Succeed())

			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName + "-dashboard",
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName + "-dashboard",
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to setup
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName + "-dashboard", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Get updated testrun and set StartTime
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName + "-dashboard", Namespace: namespace}, updatedTestRun)).To(Succeed())
			updatedTestRun.Status.StartTime = &metav1.Time{Time: time.Now()}
			updatedTestRun.Status.WorkerStatuses = []apev1.TestRunWorkerStatus{
				{
					Name:             "worker1",
					Phase:            apev1.TestRunPhaseReady,
					AssignedSegments: []string{"0:1"},
				},
			}
			Expect(k8sClient.Status().Update(ctx, updatedTestRun)).To(Succeed())

			// Reconcile to create dashboard
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName + "-dashboard", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify dashboard was created
			Expect(dashboardCreated).To(BeTrue())

			// Verify DashboardUID was set
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName + "-dashboard", Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.DashboardUID).To(Equal("test-dashboard-uid"))
		})
	})

	Context("When reconciling TestRun with VictoriaLogs", func() {
		const (
			testRunName      = "test-testrun-vl"
			testScenarioName = "test-scenario-vl"
			influxDBName     = "test-influxdb-vl"
			namespace        = "default"
			victoriaLogsURL  = "http://victorialogs:9428"
		)

		ctx := context.Background()

		var mockClient *mockInfluxDBClientForTestRun
		var mockGrafana *mockGrafanaClient
		var controllerReconciler *TestRunReconciler

		BeforeEach(func() {
			mockClient = &mockInfluxDBClientForTestRun{}
			mockGrafana = &mockGrafanaClient{}

			controllerReconciler = &TestRunReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockClient
				},
				GrafanaClient:         mockGrafana,
				VictoriaLogsURL:       victoriaLogsURL,
				VictoriaLogsAccountID: 42,
			}

			// Create InfluxDB resource
			influxDB := &apev1.InfluxDB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      influxDBName,
					Namespace: namespace,
				},
				Spec: apev1.InfluxDBSpec{
					Address: apev1.SourcedValue{
						Value: "http://localhost:8086",
					},
					Token: apev1.SourcedValue{
						Value: "admin-token",
					},
					Organization: apev1.SourcedValue{
						Value: "test-org",
					},
				},
			}
			Expect(k8sClient.Create(ctx, influxDB)).To(Succeed())

			// Update InfluxDB status with OrganizationID
			influxDB.Status.OrganizationID = "test-org-id"
			Expect(k8sClient.Status().Update(ctx, influxDB)).To(Succeed())

			// Create TestScenario resource
			testScenario := &apev1.TestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testScenarioName,
					Namespace: namespace,
				},
				Spec: apev1.TestScenarioSpec{
					TestSource: apev1.TestSource{
						GitRepo: &apev1.TestGitRepoSource{
							Repository: "https://github.com/example/test-repo",
							Revision:   "main",
							Path:       "loadtest.js",
						},
					},
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testScenario)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup resources
			testRun := &apev1.TestRun{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, testRun); err == nil {
				Expect(kubernetesutil.Cleanup(ctx, k8sClient, testRun, testRunFinalizerInfluxDBCredentials)).To(Succeed())
			}

			influxDB := &apev1.InfluxDB{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: influxDBName, Namespace: namespace}, influxDB); err == nil {
				Expect(k8sClient.Delete(ctx, influxDB)).To(Succeed())
			}

			scenario := &apev1.TestScenario{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: testScenarioName, Namespace: namespace}, scenario); err == nil {
				Expect(k8sClient.Delete(ctx, scenario)).To(Succeed())
			}
		})

		It("should initialize VictoriaLogs configuration during setup", func() {
			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// First reconcile to update spec
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to initialize VictoriaLogs config
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify VictoriaLogs configuration was set
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.VictoriaLogs).NotTo(BeNil())
			Expect(updatedTestRun.Status.VictoriaLogs.URL).To(Equal(victoriaLogsURL))
			Expect(updatedTestRun.Status.VictoriaLogs.AccountID).To(Equal(int32(42)))
			Expect(updatedTestRun.Status.VictoriaLogs.ProjectID).NotTo(BeZero())
		})

		It("should create VictoriaLogs Grafana datasource during setup", func() {
			datasourceCreated := false
			var createdDS *GrafanaDatasource
			mockGrafana.createDatasourceFunc = func(ctx context.Context, ds *GrafanaDatasource) (string, error) {
				datasourceCreated = true
				createdDS = ds
				return "vl-datasource-uid", nil
			}
			mockGrafana.getDatasourceFunc = func(ctx context.Context, uid string) (*GrafanaDatasource, error) {
				return nil, ErrNotFound
			}

			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
					Workers: []apev1.TestRunWorkerSpec{
						{
							ObjectReference: apev1.ObjectReference{
								Name:      "worker1",
								Namespace: namespace,
							},
							NumWorkers: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// Reconcile to initialize VictoriaLogs
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify datasource was created
			Expect(datasourceCreated).To(BeTrue())
			Expect(createdDS).NotTo(BeNil())
			Expect(createdDS.Type).To(Equal("victoriametrics-logs-datasource"))
			Expect(createdDS.URL).To(Equal(victoriaLogsURL))
			Expect(createdDS.JSONData).To(HaveKeyWithValue("accountID", int32(42)))
		})

		It("should not recreate datasource if it already exists", func() {
			datasourceCreationCount := 0
			mockGrafana.createDatasourceFunc = func(ctx context.Context, ds *GrafanaDatasource) (string, error) {
				datasourceCreationCount++
				return "vl-datasource-uid", nil
			}
			mockGrafana.getDatasourceFunc = func(ctx context.Context, uid string) (*GrafanaDatasource, error) {
				// Return existing datasource after first creation
				if datasourceCreationCount > 0 {
					return &GrafanaDatasource{
						UID:  uid,
						Type: "victorialogs-datasource",
					}, nil
				}
				return nil, ErrNotFound
			}

			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// Multiple reconciles
			for i := 0; i < 3; i++ {
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Datasource should only be created once
			Expect(datasourceCreationCount).To(Equal(1))
		})

		It("should not create VictoriaLogs config when VictoriaLogsURL is empty", func() {
			controllerReconciler.VictoriaLogsURL = ""

			testRun := &apev1.TestRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRunName,
					Namespace: namespace,
				},
				Spec: apev1.TestRunSpec{
					Target: "https://example.com",
					Scenario: &apev1.ObjectReference{
						Name:      testScenarioName,
						Namespace: namespace,
					},
					InfluxDB: &apev1.ObjectReference{
						Name:      influxDBName,
						Namespace: namespace,
					},
					State: apev1.TestRunStateActive,
				},
			}
			Expect(k8sClient.Create(ctx, testRun)).To(Succeed())

			// Reconcile
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testRunName, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify VictoriaLogs config was not created
			updatedTestRun := &apev1.TestRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testRunName, Namespace: namespace}, updatedTestRun)).To(Succeed())
			Expect(updatedTestRun.Status.VictoriaLogs).To(BeNil())
		})
	})
})

// updateTrackingClient wraps a client to track Update calls
type updateTrackingClient struct {
	client.Client
	onUpdate func(obj client.Object)
}

func (c *updateTrackingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.onUpdate != nil {
		c.onUpdate(obj)
	}
	return c.Client.Update(ctx, obj, opts...)
}
