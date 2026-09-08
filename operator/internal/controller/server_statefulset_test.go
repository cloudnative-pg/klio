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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// --- StatefulSet reconciliation tests ---

func newTestServerForStatefulSet() *kliov1alpha1.Server {
	return &kliov1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: kliov1alpha1.ServerSpec{
			ImageConfiguration: kliov1alpha1.ImageConfiguration{
				Image: "klio:test",
			},
			TLSConfiguration: kliov1alpha1.TLSConfiguration{
				TLSSecretName:      "tls-secret",
				ClientCASecretName: "ca-secret",
			},
			Mode:    kliov1alpha1.ModeStandard,
			Storage: kliov1alpha1.Storage{PersistentVolumeClaimTemplate: newPVCSpec("10Gi")},
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
			},
		},
	}
}

func newTestTier2Configuration() *kliov1alpha1.Tier2Configuration {
	return &kliov1alpha1.Tier2Configuration{
		S3:                &kliov1alpha1.S3Configuration{BucketName: "test-bucket"},
		EncryptionKeyFile: newTestFileSource("tier2-enc-secret", "encryption-key.age"),
		IdentityFile:      newTestFileSource("tier2-id-secret", "identity.txt"),
	}
}

// TestReconcileStatefulSetUnifiedPVC asserts that every server, whatever its
// tier configuration, gets exactly one VolumeClaimTemplate mounted at /klio.
func TestReconcileStatefulSetUnifiedPVC(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*kliov1alpha1.Server)
	}{
		{
			name:   "tier1 only",
			mutate: func(_ *kliov1alpha1.Server) {},
		},
		{
			name: "both tiers",
			mutate: func(server *kliov1alpha1.Server) {
				server.Spec.Tier2 = newTestTier2Configuration()
			},
		},
		{
			name: "tier2 only, read-only",
			mutate: func(server *kliov1alpha1.Server) {
				server.Spec.Mode = kliov1alpha1.ModeReadOnly
				server.Spec.Tier1 = nil
				server.Spec.Tier2 = newTestTier2Configuration()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServerForStatefulSet()
			tc.mutate(server)

			scheme := newTestScheme()
			require.NoError(t, appsv1.AddToScheme(scheme))
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server).Build()
			reconciler := &ServerReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: &events.FakeRecorder{Events: make(chan string, 10)},
			}

			result, err := reconciler.reconcileStatefulSet(context.Background(), server)
			require.NoError(t, err)
			assert.True(t, result.IsZero())

			var statefulSet appsv1.StatefulSet
			require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKey{
				Name: "test-server-klio", Namespace: "default",
			}, &statefulSet))

			require.Len(t, statefulSet.Spec.VolumeClaimTemplates, 1)
			pvc := statefulSet.Spec.VolumeClaimTemplates[0]
			assert.Equal(t, "klio", pvc.Name)
			assert.Equal(t, "klio", pvc.Labels[pvcTypeLabel])
			assert.Equal(t, "test-server", pvc.Labels[klioServerLabel])
			assert.Equal(t, newPVCSpec("10Gi"), pvc.Spec)

			mounts := statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts
			assert.Contains(t, mounts, corev1.VolumeMount{Name: "klio", MountPath: "/klio"})
		})
	}
}

func newInvalidStatefulSetError() error {
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "apps", Kind: "StatefulSet"},
		"test-server-klio",
		nil,
	)
}

// TestReconcileStatefulSetInvalidSpecNeverCreated tests that when a StatefulSet
// fails validation during creation (never existed), the original error is returned
// directly instead of attempting to delete a non-existent object.
func TestReconcileStatefulSetInvalidSpecNeverCreated(t *testing.T) {
	server := newTestServerForStatefulSet()
	scheme := newTestScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Create a client that returns Invalid error on StatefulSet creation
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(server).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*appsv1.StatefulSet); ok {
					return newInvalidStatefulSetError()
				}

				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	reconciler := &ServerReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: &events.FakeRecorder{Events: make(chan string, 10)},
	}

	result, err := reconciler.reconcileStatefulSet(context.Background(), server)

	require.Error(t, err)
	assert.True(t, apierrors.IsInvalid(err), "expected Invalid error, got: %v", err)
	assert.True(t, result.IsZero(), "should not request requeue on validation error")
}

// TestReconcileStatefulSetInvalidSpecExistingStatefulSet tests that when a StatefulSet
// update fails due to immutable field changes, the controller deletes it for recreation.
func TestReconcileStatefulSetInvalidSpecExistingStatefulSet(t *testing.T) {
	server := newTestServerForStatefulSet()
	scheme := newTestScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	// Create an existing StatefulSet with a non-zero CreationTimestamp
	existingStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-server-klio",
			Namespace:         "default",
			CreationTimestamp: metav1.Time{Time: time.Now()},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: kliov1alpha1.GroupVersion.String(),
					Kind:       "Server",
					Name:       server.Name,
					UID:        server.UID,
					Controller: new(true),
				},
			},
			Annotations: map[string]string{
				"klio.cnpg.io/klio-server-hash": "old-hash",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: server.Name + "-klio",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					klioServerLabel: server.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						klioServerLabel: server.Name,
					},
				},
			},
		},
	}

	var deleteWasCalled bool

	// Create a client that returns Invalid error on StatefulSet update
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(server, existingStatefulSet).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*appsv1.StatefulSet); ok {
					return newInvalidStatefulSetError()
				}

				return c.Update(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*appsv1.StatefulSet); ok {
					deleteWasCalled = true
				}

				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()

	reconciler := &ServerReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: &events.FakeRecorder{Events: make(chan string, 10)},
	}

	result, err := reconciler.reconcileStatefulSet(context.Background(), server)

	// Should succeed (no error) and request requeue for recreation
	require.NoError(t, err)
	assert.True(t, deleteWasCalled, "StatefulSet should have been deleted for recreation")
	assert.Equal(t, time.Second, result.RequeueAfter, "should requeue after 1 second for recreation")
}

func TestServerPodSecurityContext(t *testing.T) {
	t.Run("returns full security context on vanilla Kubernetes", func(t *testing.T) {
		r := &ServerReconciler{HaveSecurityContextConstraints: false}
		sc := r.serverPodSecurityContext()
		require.NotNil(t, sc)
		assert.Equal(t, int64(1000), *sc.RunAsUser)
		assert.Equal(t, int64(1000), *sc.RunAsGroup)
		assert.Equal(t, int64(1000), *sc.FSGroup)
		assert.True(t, *sc.RunAsNonRoot)
		assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)
	})

	t.Run("returns nil on OpenShift", func(t *testing.T) {
		r := &ServerReconciler{HaveSecurityContextConstraints: true}
		sc := r.serverPodSecurityContext()
		assert.Nil(t, sc)
	})
}
