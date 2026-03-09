package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
	"github.com/cloudnative-pg/klio/operator/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PluginConfiguration Controller", func() {
	ctx := context.Background()

	Context("When a PluginConfiguration is reconciled", func() {
		const (
			pcName    = "test-pc-reconcile"
			namespace = "default"
		)

		var pc *kliov1alpha1.PluginConfiguration

		BeforeEach(func() {
			pc = &kliov1alpha1.PluginConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pcName,
					Namespace: namespace,
				},
				Spec: kliov1alpha1.PluginConfigurationSpec{
					ServerAddress:    "klio-server.default",
					ClientSecretName: "client-tls",
					ServerSecretName: "server-tls",
					ClusterName:      "test-cluster",
					Mode:             kliov1alpha1.ModeStandard,
					Tier2: &kliov1alpha1.Tier2PluginConfiguration{
						EnableBackup:   true,
						EnableRecovery: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, pc)
			_ = k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pcName,
					Namespace: namespace,
				},
			})
		})

		It("should create a secret named after the PC with config.yaml data key", func() {
			reconciler := &PluginConfigurationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &secret)).To(Succeed())

			Expect(secret.Data).To(HaveKey(klioconfig.ConfigDataKey))
			Expect(secret.Labels).To(HaveKeyWithValue(
				klioconfig.TypeLabelKey, klioconfig.TypeLabelValue))

			var generatedConfig config.Data
			Expect(yaml.Unmarshal(secret.Data[klioconfig.ConfigDataKey], &generatedConfig)).To(Succeed())
			Expect(generatedConfig.Tier2BackupEnabled).To(BeTrue())
			Expect(generatedConfig.Tier1Enabled).To(BeTrue())
			Expect(generatedConfig.Client.ClusterName).To(Equal("test-cluster"))
		})

		It("should update the secret when reconciled again", func() {
			reconciler := &PluginConfigurationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile creates the secret
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Update the PC spec
			var latestPC kliov1alpha1.PluginConfiguration
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &latestPC)).To(Succeed())
			latestPC.Spec.Tier2.EnableBackup = false
			Expect(k8sClient.Update(ctx, &latestPC)).To(Succeed())

			// Second reconcile updates the secret
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &secret)).To(Succeed())

			var generatedConfig config.Data
			Expect(yaml.Unmarshal(secret.Data[klioconfig.ConfigDataKey], &generatedConfig)).To(Succeed())
			Expect(generatedConfig.Tier2BackupEnabled).To(BeFalse())
		})

		It("should set ConfigurationApplied status condition on create", func() {
			reconciler := &PluginConfigurationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Check the status condition was set
			var latestPC kliov1alpha1.PluginConfiguration
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &latestPC)).To(Succeed())

			Expect(latestPC.Status.Conditions).To(HaveLen(1))
			condition := latestPC.Status.Conditions[0]
			Expect(condition.Type).To(Equal(kliov1alpha1.PluginConfigurationConditionConfigurationApplied))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(kliov1alpha1.ReasonSecretUpdated))
			Expect(condition.ObservedGeneration).To(Equal(latestPC.Generation))
		})

		It("should set owner reference on created secret", func() {
			reconciler := &PluginConfigurationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &secret)).To(Succeed())

			// Verify owner reference is set
			Expect(secret.OwnerReferences).To(HaveLen(1))
			Expect(secret.OwnerReferences[0].Name).To(Equal(pcName))
			Expect(secret.OwnerReferences[0].Kind).To(Equal("PluginConfiguration"))
			Expect(*secret.OwnerReferences[0].Controller).To(BeTrue())
		})

		It("should not update status when secret is unchanged", func() {
			reconciler := &PluginConfigurationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile creates the secret
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Get the initial status
			var pcAfterFirst kliov1alpha1.PluginConfiguration
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &pcAfterFirst)).To(Succeed())
			initialConditionTime := pcAfterFirst.Status.Conditions[0].LastTransitionTime

			// Second reconcile without any changes
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// The status should not have been updated (same condition time)
			var pcAfterSecond kliov1alpha1.PluginConfiguration
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &pcAfterSecond)).To(Succeed())

			// Condition should still be there with same timestamp
			Expect(pcAfterSecond.Status.Conditions).To(HaveLen(1))
			Expect(pcAfterSecond.Status.Conditions[0].LastTransitionTime).To(Equal(initialConditionTime))
		})

		It("should update ObservedGeneration when PC spec changes", func() {
			reconciler := &PluginConfigurationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Get initial generation
			var pcInitial kliov1alpha1.PluginConfiguration
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &pcInitial)).To(Succeed())
			initialGeneration := pcInitial.Generation

			// Update the PC spec to bump generation
			pcInitial.Spec.Tier2.EnableBackup = false
			Expect(k8sClient.Update(ctx, &pcInitial)).To(Succeed())

			// Reconcile again
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Check ObservedGeneration was updated
			var pcFinal kliov1alpha1.PluginConfiguration
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pcName,
				Namespace: namespace,
			}, &pcFinal)).To(Succeed())

			Expect(pcFinal.Generation).To(BeNumerically(">", initialGeneration))
			Expect(pcFinal.Status.Conditions[0].ObservedGeneration).To(Equal(pcFinal.Generation))
		})
	})

	Context("When the PluginConfiguration is deleted", func() {
		It("should be a no-op", func() {
			reconciler := &PluginConfigurationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "deleted-pc",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})
