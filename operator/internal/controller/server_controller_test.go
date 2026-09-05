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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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
						Tier1: &kliov1alpha1.Tier1Configuration{
							Cache: &kliov1alpha1.Cache{
								PersistentVolumeClaimTemplate: pvcTemplate,
							},
							Data: kliov1alpha1.Data{
								PersistentVolumeClaimTemplate: pvcTemplate,
							},
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
						Queue: &kliov1alpha1.Queue{
							PersistentVolumeClaimTemplate: pvcTemplate,
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
	})
})
