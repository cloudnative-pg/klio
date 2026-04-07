package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// --- test factories ---

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = kliov1alpha1.AddToScheme(scheme)

	return scheme
}

func newPVCSpec(size string) corev1.PersistentVolumeClaimSpec {
	return corev1.PersistentVolumeClaimSpec{
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
		},
	}
}

func newTestPVC(name, serverName, pvcType, size string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				klioServerLabel: serverName,
				pvcTypeLabel:    pvcType,
			},
		},
		Spec: newPVCSpec(size),
	}
}

//nolint:ireturn
func newTestReconciler(objs ...client.Object) (*ServerReconciler, client.Client) {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	return &ServerReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: &events.FakeRecorder{Events: make(chan string, 10)},
	}, fakeClient
}

func newTestServerTier1(dataSize, cacheSize string) *kliov1alpha1.Server {
	return &kliov1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{Name: "test-server", Namespace: "default"},
		Spec: kliov1alpha1.ServerSpec{
			Tier1: &kliov1alpha1.Tier1Configuration{
				Data:  kliov1alpha1.Data{PersistentVolumeClaimTemplate: newPVCSpec(dataSize)},
				Cache: kliov1alpha1.Cache{PersistentVolumeClaimTemplate: newPVCSpec(cacheSize)},
			},
		},
	}
}

// --- buildDesiredPVCSizes tests ---

func TestBuildDesiredPVCSizes_Tier1Only(t *testing.T) {
	server := newTestServerTier1("100Gi", "10Gi")
	server.Spec.Queue = &kliov1alpha1.Queue{PersistentVolumeClaimTemplate: newPVCSpec("5Gi")}

	sizes := (&ServerReconciler{}).buildDesiredPVCSizes(server)

	require.Len(t, sizes, 3)
	assert.Equal(t, resource.MustParse("100Gi"), sizes[pvcTypeData])
	assert.Equal(t, resource.MustParse("10Gi"), sizes[pvcTypeCacheTier1])
	assert.Equal(t, resource.MustParse("5Gi"), sizes[pvcTypeQueue])
}

func TestBuildDesiredPVCSizes_Tier2Only(t *testing.T) {
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			Mode: kliov1alpha1.ModeReadOnly,
			Tier2: &kliov1alpha1.Tier2Configuration{
				Cache: kliov1alpha1.Cache{PersistentVolumeClaimTemplate: newPVCSpec("20Gi")},
				S3:    &kliov1alpha1.S3Configuration{BucketName: "test-bucket"},
			},
		},
	}

	sizes := (&ServerReconciler{}).buildDesiredPVCSizes(server)

	require.Len(t, sizes, 1)
	assert.Equal(t, resource.MustParse("20Gi"), sizes[pvcTypeCacheTier2])
}

func TestBuildDesiredPVCSizes_BothTiers(t *testing.T) {
	server := newTestServerTier1("100Gi", "10Gi")
	server.Spec.Tier2 = &kliov1alpha1.Tier2Configuration{
		Cache: kliov1alpha1.Cache{PersistentVolumeClaimTemplate: newPVCSpec("20Gi")},
		S3:    &kliov1alpha1.S3Configuration{BucketName: "test-bucket"},
	}
	server.Spec.Queue = &kliov1alpha1.Queue{PersistentVolumeClaimTemplate: newPVCSpec("5Gi")}

	sizes := (&ServerReconciler{}).buildDesiredPVCSizes(server)

	require.Len(t, sizes, 4)
	assert.Equal(t, resource.MustParse("100Gi"), sizes[pvcTypeData])
	assert.Equal(t, resource.MustParse("10Gi"), sizes[pvcTypeCacheTier1])
	assert.Equal(t, resource.MustParse("20Gi"), sizes[pvcTypeCacheTier2])
	assert.Equal(t, resource.MustParse("5Gi"), sizes[pvcTypeQueue])
}

func TestBuildDesiredPVCSizes_EmptyServer(t *testing.T) {
	sizes := (&ServerReconciler{}).buildDesiredPVCSizes(&kliov1alpha1.Server{})
	assert.Empty(t, sizes)
}

func TestBuildDesiredPVCSizes_NoStorageRequests(t *testing.T) {
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			Tier1: &kliov1alpha1.Tier1Configuration{},
		},
	}

	sizes := (&ServerReconciler{}).buildDesiredPVCSizes(server)
	assert.Empty(t, sizes)
}

// --- reconcilePVCResizes tests ---

func TestReconcilePVCResizes(t *testing.T) {
	testCases := []struct {
		name              string
		pvcCurrentSize    string
		serverDesiredSize string
		expectedPVCSize   string
	}{
		{
			name:              "expands PVC",
			pvcCurrentSize:    "10Gi",
			serverDesiredSize: "20Gi",
			expectedPVCSize:   "20Gi",
		},
		{
			name:              "skips when size is equal",
			pvcCurrentSize:    "10Gi",
			serverDesiredSize: "10Gi",
			expectedPVCSize:   "10Gi",
		},
		{
			name:              "skips shrinking",
			pvcCurrentSize:    "20Gi",
			serverDesiredSize: "10Gi",
			expectedPVCSize:   "20Gi",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pvc := newTestPVC("data-test-server-klio-0", "test-server", pvcTypeData, tc.pvcCurrentSize)
			reconciler, fakeClient := newTestReconciler(pvc)
			server := newTestServerTier1(tc.serverDesiredSize, "5Gi")

			result, err := reconciler.reconcilePVCResizes(context.Background(), server)
			require.NoError(t, err)
			assert.True(t, result.IsZero(), "should not requeue")

			var updatedPVC corev1.PersistentVolumeClaim
			require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKey{
				Name: "data-test-server-klio-0", Namespace: "default",
			}, &updatedPVC))

			expectedSize := resource.MustParse(tc.expectedPVCSize)
			actualSize := updatedPVC.Spec.Resources.Requests[corev1.ResourceStorage]
			assert.Equal(t, 0, expectedSize.Cmp(actualSize),
				"PVC size mismatch: expected %s, got %s", tc.expectedPVCSize, actualSize.String())
		})
	}
}

func TestReconcilePVCResizes_NoPVCsExist(t *testing.T) {
	reconciler, _ := newTestReconciler()
	server := newTestServerTier1("20Gi", "5Gi")

	result, err := reconciler.reconcilePVCResizes(context.Background(), server)
	require.NoError(t, err)
	assert.True(t, result.IsZero())
}

func TestReconcilePVCResizes_OrphanedPVCIgnored(t *testing.T) {
	// PVC for tier2 cache exists, but server only has tier1
	pvc := newTestPVC("cachetier2-test-server-klio-0", "test-server", pvcTypeCacheTier2, "10Gi")
	reconciler, fakeClient := newTestReconciler(pvc)
	server := newTestServerTier1("20Gi", "5Gi")

	result, err := reconciler.reconcilePVCResizes(context.Background(), server)
	require.NoError(t, err)
	assert.True(t, result.IsZero())

	var updatedPVC corev1.PersistentVolumeClaim
	require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKey{
		Name: "cachetier2-test-server-klio-0", Namespace: "default",
	}, &updatedPVC))
	expectedSize := resource.MustParse("10Gi")
	actualSize := updatedPVC.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, 0, expectedSize.Cmp(actualSize),
		"orphaned tier2 PVC should not be modified")
}

// --- findServerForPVC tests ---

func TestFindServerForPVC(t *testing.T) {
	testCases := []struct {
		name           string
		obj            client.Object
		expectedLen    int
		expectedServer string
	}{
		{
			name: "returns request for PVC with klio-server label",
			obj: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-my-server-klio-0",
					Namespace: "test-ns",
					Labels:    map[string]string{klioServerLabel: "my-server"},
				},
			},
			expectedLen:    1,
			expectedServer: "my-server",
		},
		{
			name: "returns empty for PVC without klio-server label",
			obj: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-pvc",
					Namespace: "test-ns",
					Labels:    map[string]string{},
				},
			},
			expectedLen: 0,
		},
		{
			name: "returns empty for non-PVC object",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-configmap",
					Namespace: "test-ns",
				},
			},
			expectedLen: 0,
		},
	}

	reconciler := &ServerReconciler{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requests := reconciler.findServerForPVC(context.Background(), tc.obj)
			assert.Len(t, requests, tc.expectedLen)
			if tc.expectedLen > 0 {
				assert.Equal(t, tc.expectedServer, requests[0].Name)
				assert.Equal(t, tc.obj.GetNamespace(), requests[0].Namespace)
			}
		})
	}
}

// --- isVolumeExpansionError tests ---

func TestIsVolumeExpansionError(t *testing.T) {
	pvcGR := schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"}
	pvcGK := schema.GroupKind{Group: "", Kind: "PersistentVolumeClaim"}

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "invalid error without volume expansion message",
			err:      apierrors.NewInvalid(pvcGK, "test-pvc", nil),
			expected: false,
		},
		{
			name:     "invalid error with 'does not support volume expansion' message",
			err:      newInvalidErrorWithMessage("does not support volume expansion"),
			expected: true,
		},
		{
			name:     "invalid error with 'allowVolumeExpansion' message",
			err:      newInvalidErrorWithMessage("StorageClass allowVolumeExpansion is false"),
			expected: true,
		},
		{
			name:     "forbidden error with volume expansion message",
			err:      apierrors.NewForbidden(pvcGR, "test-pvc", errors.New("does not support volume expansion")),
			expected: true,
		},
		{
			name:     "forbidden error without volume expansion message",
			err:      apierrors.NewForbidden(pvcGR, "test-pvc", errors.New("access denied")),
			expected: false,
		},
		{
			name:     "not found error",
			err:      apierrors.NewNotFound(pvcGR, "test-pvc"),
			expected: false,
		},
		{
			name:     "generic error with volume expansion message",
			err:      errors.New("does not support volume expansion"),
			expected: false, // Not an API error type
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isVolumeExpansionError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// newInvalidErrorWithMessage creates an Invalid API error with a custom message.
func newInvalidErrorWithMessage(message string) *apierrors.StatusError {
	return &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Reason:  metav1.StatusReasonInvalid,
			Message: message,
		},
	}
}

// --- expandPVC tests ---

func TestExpandPVC(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pvc", Namespace: "default"},
		Spec:       newPVCSpec("10Gi"),
	}
	reconciler, fakeClient := newTestReconciler(pvc)

	currentSize := resource.MustParse("10Gi")
	desiredSize := resource.MustParse("20Gi")

	err := reconciler.expandPVC(context.Background(), pvc, desiredSize, currentSize)
	require.NoError(t, err)

	var updatedPVC corev1.PersistentVolumeClaim
	require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKey{
		Name: "test-pvc", Namespace: "default",
	}, &updatedPVC))
	expectedSize := resource.MustParse("20Gi")
	actualSize := updatedPVC.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, 0, expectedSize.Cmp(actualSize),
		"PVC size mismatch: expected %s, got %s", expectedSize.String(), actualSize.String())
}
