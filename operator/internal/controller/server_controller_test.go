/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const serverCRDName = "servers.klio.cnpg.io"

var _ = Describe("Server Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		server := &kliov1alpha1.Server{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Server")
			err := k8sClient.Get(ctx, typeNamespacedName, server)
			if err != nil && errors.IsNotFound(err) {
				pvcTemplate := corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				}
				resource := &kliov1alpha1.Server{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: kliov1alpha1.ServerSpec{
						ImageConfiguration: kliov1alpha1.ImageConfiguration{
							Image: "klio:test",
						},
						TLSConfiguration: kliov1alpha1.TLSConfiguration{
							TLSSecretName:      "tls-secret",
							ClientCASecretName: "ca-secret",
						},
						Mode: kliov1alpha1.ModeStandard,
						Storage: kliov1alpha1.Storage{
							PersistentVolumeClaimTemplate: pvcTemplate,
						},
						Tier1: &kliov1alpha1.Tier1Configuration{
							EncryptionKeyFile: kliov1alpha1.FileSource{
								FileReference: &kliov1alpha1.FileReference{
									Volume: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{SecretName: "enc-secret"},
									},
									Path: "encryption-key.age",
								},
							},
							IdentityFile: kliov1alpha1.FileSource{
								FileReference: &kliov1alpha1.FileReference{
									Volume: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{SecretName: "id-secret"},
									},
									Path: "identity.txt",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &kliov1alpha1.Server{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Server")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ServerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: &events.FakeRecorder{Events: make(chan string, 10)},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow growing the storage PVC but reject shrinking it", func() {
			resize := func(size string) error {
				cur := &kliov1alpha1.Server{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, cur)).To(Succeed())
				requests := cur.Spec.Storage.PersistentVolumeClaimTemplate.Resources.Requests
				requests[corev1.ResourceStorage] = resource.MustParse(size)

				return k8sClient.Update(ctx, cur)
			}

			Expect(resize("2Gi")).To(Succeed(), "growing must be allowed")
			Expect(resize("2Gi")).To(Succeed(), "keeping the same size must be allowed")

			err := resize("1Gi")
			Expect(err).To(HaveOccurred(), "shrinking must be rejected")
			Expect(err.Error()).To(ContainSubstring("storage PVC size cannot be decreased"))
		})

		It("should not error when adding spec.storage to a Server whose stored object lacks it", func() {
			// spec.storage is required on write, but the required-field check
			// does not apply retroactively: a Server stored while an older
			// CRD schema was active can still be read back without it. The
			// storage-shrink CEL rule runs on every update and reads
			// oldSelf.storage, so it must handle that object shape without
			// erroring.
			storageName := "server-without-storage"
			storageKey := types.NamespacedName{Name: storageName, Namespace: "default"}

			By("relaxing the CRD's required fields so a Server without spec.storage can be created")
			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverCRDName}, crd)).To(Succeed())

			versionIdx := -1
			for i := range crd.Spec.Versions {
				if crd.Spec.Versions[i].Storage {
					versionIdx = i
					break
				}
			}
			Expect(versionIdx).To(BeNumerically(">=", 0), "no storage version found for the Server CRD")

			specSchema := crd.Spec.Versions[versionIdx].Schema.OpenAPIV3Schema.Properties["spec"]
			originalRequired := specSchema.Required
			relaxedRequired := make([]string, 0, len(originalRequired))
			for _, field := range originalRequired {
				if field != "storage" {
					relaxedRequired = append(relaxedRequired, field)
				}
			}
			specSchema.Required = relaxedRequired
			crd.Spec.Versions[versionIdx].Schema.OpenAPIV3Schema.Properties["spec"] = specSchema
			Expect(k8sClient.Update(ctx, crd)).To(Succeed())

			DeferCleanup(func() {
				By("restoring the storage requirement on the CRD")
				restored := &apiextensionsv1.CustomResourceDefinition{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverCRDName}, restored)).To(Succeed())
				restoredSpecSchema := restored.Spec.Versions[versionIdx].Schema.OpenAPIV3Schema.Properties["spec"]
				restoredSpecSchema.Required = originalRequired
				restored.Spec.Versions[versionIdx].Schema.OpenAPIV3Schema.Properties["spec"] = restoredSpecSchema
				Expect(k8sClient.Update(ctx, restored)).To(Succeed())
			})

			By("creating a Server with no spec.storage")
			typed := &kliov1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name:      storageName,
					Namespace: "default",
				},
				Spec: kliov1alpha1.ServerSpec{
					ImageConfiguration: kliov1alpha1.ImageConfiguration{
						Image: "klio:test",
					},
					TLSConfiguration: kliov1alpha1.TLSConfiguration{
						TLSSecretName:      "tls-secret",
						ClientCASecretName: "ca-secret",
					},
					Mode: kliov1alpha1.ModeStandard,
					Tier1: &kliov1alpha1.Tier1Configuration{
						EncryptionKeyFile: kliov1alpha1.FileSource{
							FileReference: &kliov1alpha1.FileReference{
								Volume: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{SecretName: "enc-secret"},
								},
								Path: "encryption-key.age",
							},
						},
						IdentityFile: kliov1alpha1.FileSource{
							FileReference: &kliov1alpha1.FileReference{
								Volume: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{SecretName: "id-secret"},
								},
								Path: "identity.txt",
							},
						},
					},
				},
			}
			asMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(typed)
			Expect(err).NotTo(HaveOccurred())
			unstructured.RemoveNestedField(asMap, "spec", "storage")

			withoutStorage := &unstructured.Unstructured{Object: asMap}
			withoutStorage.SetGroupVersionKind(kliov1alpha1.GroupVersion.WithKind("Server"))
			Expect(k8sClient.Create(ctx, withoutStorage)).To(Succeed())

			DeferCleanup(func() {
				current := &unstructured.Unstructured{}
				current.SetGroupVersionKind(kliov1alpha1.GroupVersion.WithKind("Server"))
				Expect(k8sClient.Get(ctx, storageKey, current)).To(Succeed())
				Expect(k8sClient.Delete(ctx, current)).To(Succeed())
			})

			By("restoring the CRD's required fields")
			restored := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverCRDName}, restored)).To(Succeed())
			restoredSpecSchema := restored.Spec.Versions[versionIdx].Schema.OpenAPIV3Schema.Properties["spec"]
			restoredSpecSchema.Required = originalRequired
			restored.Spec.Versions[versionIdx].Schema.OpenAPIV3Schema.Properties["spec"] = restoredSpecSchema
			Expect(k8sClient.Update(ctx, restored)).To(Succeed())

			By("updating the Server to add spec.storage")
			current := &unstructured.Unstructured{}
			current.SetGroupVersionKind(kliov1alpha1.GroupVersion.WithKind("Server"))
			Expect(k8sClient.Get(ctx, storageKey, current)).To(Succeed())
			Expect(unstructured.SetNestedMap(current.Object, map[string]any{
				"accessModes": []any{"ReadWriteOnce"},
				"resources": map[string]any{
					"requests": map[string]any{"storage": "1Gi"},
				},
			}, "spec", "storage", "pvcTemplate")).To(Succeed())

			err = k8sClient.Update(ctx, current)
			Expect(err).NotTo(HaveOccurred(), "the storage-shrink CEL guard must not error when oldSelf has no storage")
		})
	})
})
