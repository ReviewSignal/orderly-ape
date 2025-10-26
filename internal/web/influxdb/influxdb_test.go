// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package influxdb

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
)

func TestInfluxDB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "InfluxDB Web Controller Suite")
}

var _ = Describe("InfluxDBController", func() {
	var (
		k8sClient  client.Client
		testScheme *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		testScheme = runtime.NewScheme()
		Expect(scheme.AddToScheme(testScheme)).To(Succeed())
		Expect(apev1.AddToScheme(testScheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(testScheme).
			Build()
	})

	Describe("SetControllerReference behavior", func() {
		It("should correctly set controller reference on a secret after InfluxDB is created", func() {
			By("Creating an InfluxDB resource")
			influxdb := &apev1.InfluxDB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-influxdb",
					Namespace: "default",
				},
				Spec: apev1.InfluxDBSpec{
					Address: apev1.SourcedValue{
						Value: "http://localhost:8086",
					},
					Organization: apev1.SourcedValue{
						Value: "test-org",
					},
					Token: apev1.SourcedValue{
						Value: "test-token",
					},
				},
			}
			Expect(k8sClient.Create(ctx, influxdb)).To(Succeed())

			By("Creating a secret with controller reference")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: influxdb.Name + "-creds-",
					Namespace:    influxdb.Namespace,
				},
				Data: map[string][]byte{
					"token": []byte("test-token"),
				},
				Type: corev1.SecretTypeOpaque,
			}

			// Set the controller reference
			Expect(controllerutil.SetControllerReference(influxdb, secret, testScheme)).To(Succeed())
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Verifying the controller reference is set on the secret")
			Expect(secret.OwnerReferences).To(HaveLen(1))
			ownerRef := secret.OwnerReferences[0]
			Expect(ownerRef.APIVersion).To(Equal(apev1.GroupVersion.String()))
			Expect(ownerRef.Kind).To(Equal("InfluxDB"))
			Expect(ownerRef.Name).To(Equal("test-influxdb"))
			Expect(ownerRef.UID).To(Equal(influxdb.UID))
			Expect(*ownerRef.Controller).To(BeTrue())

			By("Verifying that deleting InfluxDB would cascade delete the secret")
			// With the controller reference set, when InfluxDB is deleted,
			// the secret should be garbage collected automatically
			Expect(k8sClient.Delete(ctx, influxdb)).To(Succeed())

			// In a real cluster with garbage collection, the secret would be deleted.
			// With the fake client, we can at least verify the owner reference is correct.
		})

		It("should verify secret without controller reference is not owned", func() {
			By("Creating an InfluxDB resource")
			influxdb := &apev1.InfluxDB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-influxdb-2",
					Namespace: "default",
				},
				Spec: apev1.InfluxDBSpec{
					Address: apev1.SourcedValue{
						Value: "http://localhost:8086",
					},
					Organization: apev1.SourcedValue{
						Value: "test-org",
					},
					Token: apev1.SourcedValue{
						Value: "test-token",
					},
				},
			}
			Expect(k8sClient.Create(ctx, influxdb)).To(Succeed())

			By("Creating a secret WITHOUT controller reference")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: influxdb.Name + "-creds-",
					Namespace:    influxdb.Namespace,
				},
				Data: map[string][]byte{
					"token": []byte("test-token"),
				},
				Type: corev1.SecretTypeOpaque,
			}

			// Do NOT set controller reference
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Verifying NO controller reference is set on the secret")
			Expect(secret.OwnerReferences).To(BeEmpty())
		})
	})
})
