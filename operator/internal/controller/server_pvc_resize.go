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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// pvcTypeKlio is the name of the VolumeClaimTemplate backing the Server.
const pvcTypeKlio = "klio"

// klioPVCName returns the name of the PVC backing a Server. The
// StatefulSet always runs a single replica, so the ordinal is always 0.
func klioPVCName(server *kliov1alpha1.Server) string {
	return fmt.Sprintf("%s-%s-0", pvcTypeKlio, server.GetStatefulSetName())
}

// reconcilePVCResize handles resizing the PVC backing the Server.
// StatefulSet VolumeClaimTemplates are immutable, so we must patch PVCs directly.
// Note: Only expansion is supported; shrinking PVCs is not possible in Kubernetes.
//
//nolint:unparam // Result is always zero but signature matches reconciler pattern for consistency.
func (r *ServerReconciler) reconcilePVCResize(ctx context.Context, server *kliov1alpha1.Server) (ctrl.Result, error) {
	desiredSize, ok := server.Spec.Storage.PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage]
	if !ok {
		return ctrl.Result{}, nil
	}

	var pvc corev1.PersistentVolumeClaim
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: klioPVCName(server)}, &pvc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to get PVC: %w", err)
	}

	contextLogger := logf.FromContext(ctx)
	currentSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]

	switch {
	case desiredSize.Cmp(currentSize) < 0:
		contextLogger.Info("PVC shrinking is not supported, ignoring size decrease",
			"pvc", pvc.Name,
			"currentSize", currentSize.String(),
			"desiredSize", desiredSize.String())

		return ctrl.Result{}, nil
	case desiredSize.Cmp(currentSize) == 0:
		return ctrl.Result{}, nil
	}

	if err := r.expandPVC(ctx, &pvc, desiredSize, currentSize); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(server, nil, corev1.EventTypeNormal, "PVCExpanded",
		"ResizePVC", "PVC %s expanded from %s to %s", pvc.Name, currentSize.String(), desiredSize.String())

	return ctrl.Result{}, nil
}

// expandPVC patches the PVC to expand its storage size.
func (r *ServerReconciler) expandPVC(
	ctx context.Context,
	pvc *corev1.PersistentVolumeClaim,
	desiredSize, currentSize resource.Quantity,
) error {
	contextLogger := logf.FromContext(ctx)

	contextLogger.Info("Expanding PVC",
		"pvc", pvc.Name,
		"currentSize", currentSize.String(),
		"desiredSize", desiredSize.String())

	patch := client.MergeFrom(pvc.DeepCopy())
	pvc.Spec.Resources.Requests[corev1.ResourceStorage] = desiredSize

	if err := r.Patch(ctx, pvc, patch); err != nil {
		// Log helpful message if the error indicates StorageClass doesn't support expansion.
		if isVolumeExpansionError(err) {
			contextLogger.Error(err, "PVC expansion failed - StorageClass may not support volume expansion. "+
				"Ensure the StorageClass has allowVolumeExpansion: true",
				"pvc", pvc.Name,
				"storageClassName", pvc.Spec.StorageClassName)
		}

		return fmt.Errorf("failed to expand PVC %s from %s to %s: %w",
			pvc.Name, currentSize.String(), desiredSize.String(), err)
	}

	contextLogger.Info("PVC expansion applied",
		"pvc", pvc.Name,
		"oldSize", currentSize.String(),
		"newSize", desiredSize.String())

	return nil
}

// isVolumeExpansionError checks if the error indicates the StorageClass doesn't support volume expansion.
func isVolumeExpansionError(err error) bool {
	if !apierrors.IsInvalid(err) && !apierrors.IsForbidden(err) {
		return false
	}

	errMsg := err.Error()

	return strings.Contains(errMsg, "does not support volume expansion") ||
		strings.Contains(errMsg, "allowVolumeExpansion")
}

// findServerForPVC maps a PVC to its owning Server by checking the klio-server label.
func (r *ServerReconciler) findServerForPVC(_ context.Context, obj client.Object) []ctrl.Request {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil
	}

	serverName, exists := pvc.Labels[klioServerLabel]
	if !exists {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: pvc.Namespace,
				Name:      serverName,
			},
		},
	}
}
