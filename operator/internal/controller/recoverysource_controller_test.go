package controller

import (
	"context"

	machineryapi "github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RecoverySource Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-recovery-source"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		recoverySource := &kliov1alpha1.RecoverySource{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind RecoverySource")
			err := k8sClient.Get(ctx, typeNamespacedName, recoverySource)
			if err != nil && errors.IsNotFound(err) {
				resource := &kliov1alpha1.RecoverySource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: kliov1alpha1.RecoverySourceSpec{
						ImageConfiguration: kliov1alpha1.ImageConfiguration{
							Image: "registry.dev:5000/klio-testing:dev",
						},
						TLSConfiguration: kliov1alpha1.TLSConfiguration{
							TLSSecretName:      "test-tls-secret",
							ClientCASecretName: "test-ca-secret",
						},
						Tier2: kliov1alpha1.Tier2Configuration{
							EncryptionPassword: &machineryapi.SecretKeySelector{
								LocalObjectReference: machineryapi.LocalObjectReference{
									Name: "test-encryption-secret",
								},
								Key: "password",
							},
							S3: &kliov1alpha1.S3Configuration{
								BucketName: "test-bucket",
								Endpoint:   "https://minio:9000",
								Region:     "us-east-1",
								Prefix:     "klio",
							},
						},
						Storage: kliov1alpha1.RecoverySourceStorageConfiguration{
							Cache: kliov1alpha1.CacheConfiguration{
								PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
									AccessModes: []corev1.PersistentVolumeAccessMode{
										corev1.ReadWriteOnce,
									},
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("1Gi"),
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &kliov1alpha1.RecoverySource{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RecoverySource")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &RecoverySourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
