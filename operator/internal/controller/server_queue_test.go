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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// leftoverQueuePVC is the PVC a server keeps after the dedicated queue volume
// is dropped from its spec. deleting reproduces the PVC the user asked to
// remove, which the kubelet protection finalizer keeps around while the pod
// still mounts it.
func leftoverQueuePVC(deleting bool) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "queue-test-server-klio-0",
			Namespace: "default",
		},
	}
	if deleting {
		pvc.Finalizers = []string{"kubernetes.io/pvc-protection"}
		pvc.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}

	return pvc
}

// TestReconcileStatefulSetQueueLayout checks the StatefulSet the reconciler
// generates for each queue location, including the transient states where the
// dedicated volume was dropped from the spec but its PVC still holds the queue.
func TestReconcileStatefulSetQueueLayout(t *testing.T) {
	dedicated := &kliov1alpha1.Queue{PersistentVolumeClaimTemplate: newPVCSpec("1Gi")}

	tests := []struct {
		name                 string
		queue                *kliov1alpha1.Queue
		leftoverPVC          *corev1.PersistentVolumeClaim
		expectedDirectory    string
		expectedMigration    string
		expectQueueTemplate  bool
		expectQueueMount     bool
		expectQueueClaimName string
	}{
		{
			name:                "dedicated volume",
			queue:               dedicated,
			expectedDirectory:   "/queue",
			expectedMigration:   "/data/queue",
			expectQueueTemplate: true,
			expectQueueMount:    true,
		},
		{
			// The volume is templated, so a PVC left over from a previous
			// layout is the one the template owns: nothing to mount by name.
			name:                "dedicated volume, with a queue PVC already there",
			queue:               dedicated,
			leftoverPVC:         leftoverQueuePVC(false),
			expectedDirectory:   "/queue",
			expectedMigration:   "/data/queue",
			expectQueueTemplate: true,
			expectQueueMount:    true,
		},
		{
			name:              "inside the data volume",
			expectedDirectory: "/data/queue",
		},
		{
			name:                 "inside the data volume, with a leftover queue PVC",
			leftoverPVC:          leftoverQueuePVC(false),
			expectedDirectory:    "/data/queue",
			expectedMigration:    "/queue",
			expectQueueMount:     true,
			expectQueueClaimName: "queue-test-server-klio-0",
		},
		{
			// Mounting it again would block its deletion forever.
			name:              "inside the data volume, with the queue PVC being deleted",
			leftoverPVC:       leftoverQueuePVC(true),
			expectedDirectory: "/data/queue",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServerForStatefulSet()
			server.Spec.Queue = test.queue

			scheme := newTestScheme()
			require.NoError(t, appsv1.AddToScheme(scheme))

			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server)
			if test.leftoverPVC != nil {
				builder = builder.WithObjects(test.leftoverPVC)
			}

			reconciler := &ServerReconciler{
				Client:   builder.Build(),
				Scheme:   scheme,
				Recorder: &events.FakeRecorder{Events: make(chan string, 10)},
			}

			_, err := reconciler.reconcileStatefulSet(context.Background(), server)
			require.NoError(t, err)

			var statefulSet appsv1.StatefulSet
			require.NoError(t, reconciler.Get(context.Background(), types.NamespacedName{
				Namespace: "default",
				Name:      "test-server-klio",
			}, &statefulSet))

			container := statefulSet.Spec.Template.Spec.Containers[0]

			assert.Equal(t, test.expectedDirectory, envValue(container.Env, "QUEUE_DIRECTORY"))
			assert.Equal(t, test.expectedMigration, envValue(container.Env, "QUEUE_MIGRATION_SOURCE"))
			assert.Equal(t, test.expectQueueTemplate, hasVolumeClaimTemplate(&statefulSet, queueVolumeName))
			assert.Equal(t, test.expectQueueClaimName, claimName(&statefulSet, queueVolumeName))

			expectedMountPath := ""
			if test.expectQueueMount {
				expectedMountPath = queueVolumeMountPath
			}
			assert.Equal(t, expectedMountPath, mountPath(container, queueVolumeName))
		})
	}
}

// TestBuildQueueLayoutWithoutTier1 covers the only layout a StatefulSet cannot
// show: a read-only server has no queue at all.
func TestBuildQueueLayoutWithoutTier1(t *testing.T) {
	server := newTestServerForStatefulSet()
	server.Spec.Mode = kliov1alpha1.ModeReadOnly
	server.Spec.Tier1 = nil
	server.Spec.Queue = nil

	assert.Equal(t, queueLayout{}, buildQueueLayout(server, "test-server-klio", true))
}

// envValue returns the value of the named environment variable, or the empty
// string when it is not set.
func envValue(envs []corev1.EnvVar, name string) string {
	if env := findEnvVar(envs, name); env != nil {
		return env.Value
	}

	return ""
}

func hasVolumeClaimTemplate(statefulSet *appsv1.StatefulSet, name string) bool {
	for _, template := range statefulSet.Spec.VolumeClaimTemplates {
		if template.Name == name {
			return true
		}
	}

	return false
}

// mountPath returns where the named volume is mounted, or the empty string
// when the container does not mount it.
func mountPath(container corev1.Container, name string) string {
	for _, mount := range container.VolumeMounts {
		if mount.Name == name {
			return mount.MountPath
		}
	}

	return ""
}

// claimName returns the PVC the named volume refers to, or the empty string
// when the volume is absent or is not a PVC.
func claimName(statefulSet *appsv1.StatefulSet, name string) string {
	for _, volume := range statefulSet.Spec.Template.Spec.Volumes {
		if volume.Name == name && volume.PersistentVolumeClaim != nil {
			return volume.PersistentVolumeClaim.ClaimName
		}
	}

	return ""
}
