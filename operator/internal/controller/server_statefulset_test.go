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
			Mode: kliov1alpha1.ModeStandard,
			Tier1: &kliov1alpha1.Tier1Configuration{
				Data:              kliov1alpha1.Data{PersistentVolumeClaimTemplate: newPVCSpec("10Gi")},
				Cache:             kliov1alpha1.Cache{PersistentVolumeClaimTemplate: newPVCSpec("5Gi")},
				EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
			},
			Queue: &kliov1alpha1.Queue{
				PersistentVolumeClaimTemplate: newPVCSpec("1Gi"),
			},
		},
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
					typeLabel:       baseTypeLabelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						klioServerLabel: server.Name,
						typeLabel:       baseTypeLabelValue,
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
