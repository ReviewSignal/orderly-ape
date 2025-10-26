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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

// mockGrafanaClient is a mock implementation of GrafanaClient for testing
type mockGrafanaClient struct {
	createDatasourceFunc func(ctx context.Context, ds *GrafanaDatasource) (string, error)
	updateDatasourceFunc func(ctx context.Context, uid string, ds *GrafanaDatasource) error
	getDatasourceFunc    func(ctx context.Context, uid string) (*GrafanaDatasource, error)
	deleteDatasourceFunc func(ctx context.Context, uid string) error
	createDashboardFunc  func(ctx context.Context, dashboard map[string]interface{}) (string, error)
	updateDashboardFunc  func(ctx context.Context, uid string, dashboard map[string]interface{}) error
	getDashboardFunc     func(ctx context.Context, uid string) (map[string]interface{}, error)
	deleteDashboardFunc  func(ctx context.Context, uid string) error
}

func (m *mockGrafanaClient) CreateDatasource(ctx context.Context, ds *GrafanaDatasource) (string, error) {
	if m.createDatasourceFunc != nil {
		return m.createDatasourceFunc(ctx, ds)
	}
	return "test-datasource-uid", nil
}

func (m *mockGrafanaClient) UpdateDatasource(ctx context.Context, uid string, ds *GrafanaDatasource) error {
	if m.updateDatasourceFunc != nil {
		return m.updateDatasourceFunc(ctx, uid, ds)
	}
	return nil
}

func (m *mockGrafanaClient) GetDatasource(ctx context.Context, uid string) (*GrafanaDatasource, error) {
	if m.getDatasourceFunc != nil {
		return m.getDatasourceFunc(ctx, uid)
	}
	return nil, ErrNotFound
}

func (m *mockGrafanaClient) DeleteDatasource(ctx context.Context, uid string) error {
	if m.deleteDatasourceFunc != nil {
		return m.deleteDatasourceFunc(ctx, uid)
	}
	return nil
}

func (m *mockGrafanaClient) CreateDashboard(ctx context.Context, dashboard map[string]interface{}) (string, error) {
	if m.createDashboardFunc != nil {
		return m.createDashboardFunc(ctx, dashboard)
	}
	return "test-dashboard-uid", nil
}

func (m *mockGrafanaClient) UpdateDashboard(ctx context.Context, uid string, dashboard map[string]interface{}) error {
	if m.updateDashboardFunc != nil {
		return m.updateDashboardFunc(ctx, uid, dashboard)
	}
	return nil
}

func (m *mockGrafanaClient) GetDashboard(ctx context.Context, uid string) (map[string]interface{}, error) {
	if m.getDashboardFunc != nil {
		return m.getDashboardFunc(ctx, uid)
	}
	return nil, ErrNotFound
}

func (m *mockGrafanaClient) DeleteDashboard(ctx context.Context, uid string) error {
	if m.deleteDashboardFunc != nil {
		return m.deleteDashboardFunc(ctx, uid)
	}
	return nil
}

var _ = Describe("InfluxDB Grafana Controller", func() {
	Context("When reconciling an InfluxDB resource with Grafana integration", func() {
		const resourceName = "test-influxdb-grafana"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind InfluxDB")
			influxDB := &apev1.InfluxDB{}
			err := k8sClient.Get(ctx, typeNamespacedName, influxDB)
			if err != nil && errors.IsNotFound(err) {
				resource := &apev1.InfluxDB{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
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
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &apev1.InfluxDB{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance InfluxDB")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create Grafana datasource when InfluxDB is ready", func() {
			By("Setting up mock clients")
			mockInfluxClient := &mockInfluxDBClient{
				getReadyFunc: func(ctx context.Context) (string, time.Time, error) {
					return "v2.7.0", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), nil
				},
				listBucketsFunc: func(ctx context.Context) ([]string, error) {
					return []string{"bucket1", "bucket2"}, nil
				},
				createReadOnlyTokenFunc: func(ctx context.Context, orgID, description string) (string, string, error) {
					return "read-token-id", "read-token-value", nil
				},
			}

			var capturedDS *GrafanaDatasource
			mockGrafanaClient := &mockGrafanaClient{
				createDatasourceFunc: func(ctx context.Context, ds *GrafanaDatasource) (string, error) {
					capturedDS = ds
					return "grafana-ds-uid", nil
				},
			}

			By("Reconciling the created resource")
			controllerReconciler := &InfluxDBReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				GrafanaClient: mockGrafanaClient,
				GrafanaOrgID:  1,
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockInfluxClient
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking the Grafana datasource was created")
			influxDB := &apev1.InfluxDB{}
			err = k8sClient.Get(ctx, typeNamespacedName, influxDB)
			Expect(err).NotTo(HaveOccurred())

			Expect(influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasourceUID]).To(Equal("grafana-ds-uid"))
			Expect(influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaReadTokenID]).To(Equal("read-token-id"))
			// Datasource name should be generated using safename utility
			Expect(influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasource]).NotTo(BeEmpty())

			By("Checking the datasource configuration")
			Expect(capturedDS).NotTo(BeNil())
			Expect(capturedDS.Type).To(Equal("influxdb"))
			Expect(capturedDS.URL).To(Equal("http://localhost:8086"))
			Expect(capturedDS.Access).To(Equal("proxy"))
			Expect(capturedDS.SecureJSONData).To(HaveKey("token"))
			Expect(capturedDS.SecureJSONData["token"]).To(Equal("read-token-value"))
			Expect(capturedDS.JSONData["version"]).To(Equal("Flux"))
			Expect(capturedDS.JSONData["organization"]).To(Equal("test-org"))
			// defaultBucket may be empty if buckets weren't yet populated in status
			Expect(capturedDS.JSONData).To(HaveKey("defaultBucket"))
		})

		It("should not create datasource if token creation fails", func() {
			By("Setting up mock clients with failing token creation")
			mockInfluxClient := &mockInfluxDBClient{
				getReadyFunc: func(ctx context.Context) (string, time.Time, error) {
					return "v2.7.0", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), nil
				},
				listBucketsFunc: func(ctx context.Context) ([]string, error) {
					return []string{"bucket1"}, nil
				},
				createReadOnlyTokenFunc: func(ctx context.Context, orgID, description string) (string, string, error) {
					return "", "", fmt.Errorf("failed to create token")
				},
			}

			datasourceCreated := false
			mockGrafanaClient := &mockGrafanaClient{
				createDatasourceFunc: func(ctx context.Context, ds *GrafanaDatasource) (string, error) {
					datasourceCreated = true
					return "grafana-ds-uid", nil
				},
			}

			By("Reconciling the created resource")
			controllerReconciler := &InfluxDBReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				GrafanaClient: mockGrafanaClient,
				GrafanaOrgID:  1,
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockInfluxClient
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking that datasource was not created")
			Expect(datasourceCreated).To(BeFalse())
		})

		It("should skip Grafana integration when GrafanaClient is nil", func() {
			By("Setting up mock InfluxDB client")
			mockInfluxClient := &mockInfluxDBClient{
				getReadyFunc: func(ctx context.Context) (string, time.Time, error) {
					return "v2.7.0", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), nil
				},
				listBucketsFunc: func(ctx context.Context) ([]string, error) {
					return []string{"bucket1"}, nil
				},
			}

			By("Reconciling without Grafana client")
			controllerReconciler := &InfluxDBReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				GrafanaClient: nil, // No Grafana client
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockInfluxClient
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking that no Grafana resources were created")
			influxDB := &apev1.InfluxDB{}
			err = k8sClient.Get(ctx, typeNamespacedName, influxDB)
			Expect(err).NotTo(HaveOccurred())

			Expect(influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaDatasourceUID]).To(BeEmpty())
			Expect(influxDB.Annotations[apev1.InfluxDBAnnotationGrafanaReadTokenID]).To(BeEmpty())
		})

		It("should not recreate datasource on subsequent reconciliations", func() {
			By("Setting up mock clients")
			tokenCreationCount := 0
			mockInfluxClient := &mockInfluxDBClient{
				getReadyFunc: func(ctx context.Context) (string, time.Time, error) {
					return "v2.7.0", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), nil
				},
				listBucketsFunc: func(ctx context.Context) ([]string, error) {
					return []string{"bucket1"}, nil
				},
				createReadOnlyTokenFunc: func(ctx context.Context, orgID, description string) (string, string, error) {
					tokenCreationCount++
					return "read-token-id", "read-token-value", nil
				},
			}

			datasourceCreationCount := 0
			mockGrafanaClient := &mockGrafanaClient{
				createDatasourceFunc: func(ctx context.Context, ds *GrafanaDatasource) (string, error) {
					datasourceCreationCount++
					return "grafana-ds-uid", nil
				},
				getDatasourceFunc: func(ctx context.Context, uid string) (*GrafanaDatasource, error) {
					if uid == "grafana-ds-uid" {
						return &GrafanaDatasource{
							UID:  "grafana-ds-uid",
							Name: "test-datasource",
						}, nil
					}
					return nil, nil
				},
			}

			By("First reconciliation")
			controllerReconciler := &InfluxDBReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				GrafanaClient: mockGrafanaClient,
				GrafanaOrgID:  1,
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockInfluxClient
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking first reconciliation created resources")
			Expect(tokenCreationCount).To(Equal(1))
			Expect(datasourceCreationCount).To(Equal(1))

			By("Second reconciliation")
			result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking second reconciliation did not create new resources")
			Expect(tokenCreationCount).To(Equal(1))
			Expect(datasourceCreationCount).To(Equal(1))
		})
	})
})
