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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

// mockInfluxDBClient is a mock implementation of InfluxDBClient for testing
type mockInfluxDBClient struct {
	getReadyFunc            func(ctx context.Context) (string, time.Time, error)
	listBucketsFunc         func(ctx context.Context) ([]string, error)
	createTokenFunc         func(ctx context.Context, description, bucket string) (string, string, error)
	createReadOnlyTokenFunc func(ctx context.Context, orgID, description string) (string, string, error)
	deleteTokenFunc         func(ctx context.Context, tokenID string) error
}

// GetOrganizationID implements InfluxDBClient.
func (m *mockInfluxDBClient) GetOrganizationID(ctx context.Context, org string) (string, error) {
	return "test-org-id", nil
}

func (m *mockInfluxDBClient) GetReady(ctx context.Context) (string, time.Time, error) {
	if m.getReadyFunc != nil {
		return m.getReadyFunc(ctx)
	}
	return "", time.Time{}, nil
}

func (m *mockInfluxDBClient) ListBuckets(ctx context.Context) ([]string, error) {
	if m.listBucketsFunc != nil {
		return m.listBucketsFunc(ctx)
	}
	return nil, nil
}

func (m *mockInfluxDBClient) CreateToken(ctx context.Context, orgID, description, bucket string) (string, string, error) {
	if m.createTokenFunc != nil {
		return m.createTokenFunc(ctx, description, bucket)
	}
	return "", "", nil
}

func (m *mockInfluxDBClient) CreateReadOnlyToken(ctx context.Context, orgID, description string) (string, string, error) {
	if m.createReadOnlyTokenFunc != nil {
		return m.createReadOnlyTokenFunc(ctx, orgID, description)
	}
	return "", "", nil
}

func (m *mockInfluxDBClient) DeleteToken(ctx context.Context, tokenID string) error {
	if m.deleteTokenFunc != nil {
		return m.deleteTokenFunc(ctx, tokenID)
	}
	return nil
}

var _ = Describe("InfluxDB Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-influxdb"

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

		It("should successfully reconcile and set Ready condition when InfluxDB is accessible", func() {
			By("Setting up a mock client that returns success")
			mockClient := &mockInfluxDBClient{
				getReadyFunc: func(ctx context.Context) (string, time.Time, error) {
					return "v2.7.0", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), nil
				},
				listBucketsFunc: func(ctx context.Context) ([]string, error) {
					return []string{"bucket1", "bucket2"}, nil
				},
			}

			By("Reconciling the created resource")
			controllerReconciler := &InfluxDBReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockClient
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking the updated status")
			influxDB := &apev1.InfluxDB{}
			err = k8sClient.Get(ctx, typeNamespacedName, influxDB)
			Expect(err).NotTo(HaveOccurred())

			Expect(influxDB.Status.Version).To(Equal("v2.7.0"))
			Expect(influxDB.Status.ServerStartTime).NotTo(BeNil())
			expectedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			Expect(influxDB.Status.ServerStartTime.Time.Equal(expectedTime)).To(BeTrue())
			Expect(influxDB.Status.Buckets).To(ConsistOf("bucket1", "bucket2"))

			condition := meta.FindStatusCondition(influxDB.Status.Conditions, apev1.InfluxDBConditionReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(apev1.InfluxDBReasonReady))
		})

		It("should set Ready condition to False when connection fails", func() {
			By("Setting up a mock client that returns connection error")
			mockClient := &mockInfluxDBClient{
				getReadyFunc: func(ctx context.Context) (string, time.Time, error) {
					return "", time.Time{}, fmt.Errorf("connection refused")
				},
			}

			By("Reconciling the created resource")
			controllerReconciler := &InfluxDBReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockClient
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking the updated status")
			influxDB := &apev1.InfluxDB{}
			err = k8sClient.Get(ctx, typeNamespacedName, influxDB)
			Expect(err).NotTo(HaveOccurred())

			Expect(influxDB.Status.ServerStartTime).To(BeNil())

			condition := meta.FindStatusCondition(influxDB.Status.Conditions, apev1.InfluxDBConditionReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(apev1.InfluxDBReasonConnectionFailed))
		})

		It("should set Ready condition to False when listing buckets fails", func() {
			By("Setting up a mock client that fails on listing buckets")
			mockClient := &mockInfluxDBClient{
				getReadyFunc: func(ctx context.Context) (string, time.Time, error) {
					return "v2.7.0", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), nil
				},
				listBucketsFunc: func(ctx context.Context) ([]string, error) {
					return nil, fmt.Errorf("unauthorized")
				},
			}

			By("Reconciling the created resource")
			controllerReconciler := &InfluxDBReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				ClientGetter: func(address, token, organization string) InfluxDBClient {
					return mockClient
				},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Checking the updated status")
			influxDB := &apev1.InfluxDB{}
			err = k8sClient.Get(ctx, typeNamespacedName, influxDB)
			Expect(err).NotTo(HaveOccurred())

			Expect(influxDB.Status.ServerStartTime).To(BeNil())

			condition := meta.FindStatusCondition(influxDB.Status.Conditions, apev1.InfluxDBConditionReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(apev1.InfluxDBReasonListBucketsFailed))
		})

		It("should handle missing resource gracefully", func() {
			By("Reconciling a non-existent resource")
			controllerReconciler := &InfluxDBReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})
})
