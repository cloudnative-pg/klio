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

const (
	pvcTypeData       = "data"
	pvcTypeQueue      = "queue"
	pvcTypeCacheTier1 = "cachetier1"
	pvcTypeCacheTier2 = "cachetier2"
)

// reconcilePVCResizes handles PVC size expansion for all Server PVCs.
// StatefulSet VolumeClaimTemplates are immutable, so we must patch PVCs directly.
// Note: Only expansion is supported; shrinking PVCs is not possible in Kubernetes.
//
//nolint:unparam // Result is always zero but signature matches reconciler pattern for consistency.
func (r *ServerReconciler) reconcilePVCResizes(ctx context.Context, server *kliov1alpha1.Server) (ctrl.Result, error) {
	desiredSizes := r.buildDesiredPVCSizes(server)
	if len(desiredSizes) == 0 {
		return ctrl.Result{}, nil
	}

	var pvcList corev1.PersistentVolumeClaimList
	if err := r.List(ctx, &pvcList,
		client.InNamespace(server.Namespace),
		client.MatchingLabels{klioServerLabel: server.Name},
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list PVCs: %w", err)
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := r.reconcileSinglePVCResize(ctx, server, pvc, desiredSizes); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileSinglePVCResize handles the resize logic for a single PVC.
func (r *ServerReconciler) reconcileSinglePVCResize(
	ctx context.Context,
	server *kliov1alpha1.Server,
	pvc *corev1.PersistentVolumeClaim,
	desiredSizes map[string]resource.Quantity,
) error {
	contextLogger := logf.FromContext(ctx)

	pvcType, exists := pvc.Labels[pvcTypeLabel]
	if !exists {
		return nil
	}

	desiredSize, exists := desiredSizes[pvcType]
	if !exists {
		return nil
	}

	currentSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]

	switch {
	case desiredSize.Cmp(currentSize) < 0:
		contextLogger.Info("PVC shrinking is not supported, ignoring size decrease",
			"pvc", pvc.Name,
			"currentSize", currentSize.String(),
			"desiredSize", desiredSize.String())

		return nil
	case desiredSize.Cmp(currentSize) == 0:
		return nil
	}

	if err := r.expandPVC(ctx, pvc, desiredSize, currentSize); err != nil {
		return err
	}

	r.Recorder.Eventf(server, nil, corev1.EventTypeNormal, "PVCExpanded",
		"ResizePVC", "PVC %s expanded from %s to %s", pvc.Name, currentSize.String(), desiredSize.String())

	return nil
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

// buildDesiredPVCSizes returns a map of PVC type labels to their desired sizes.
func (r *ServerReconciler) buildDesiredPVCSizes(server *kliov1alpha1.Server) map[string]resource.Quantity {
	sizes := make(map[string]resource.Quantity)

	if server.Spec.Tier1 != nil {
		if size, ok := server.Spec.Tier1.Data.PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage]; ok {
			sizes[pvcTypeData] = size
		}
		if server.Spec.Tier1.Cache != nil {
			if size, ok := server.Spec.Tier1.Cache.
				PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage]; ok {
				sizes[pvcTypeCacheTier1] = size
			}
		}
	}

	if server.Spec.Tier2 != nil && server.Spec.Tier2.Cache != nil {
		if size, ok := server.Spec.Tier2.Cache.
			PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage]; ok {
			sizes[pvcTypeCacheTier2] = size
		}
	}

	if server.Spec.Queue != nil {
		if size, ok := server.Spec.Queue.PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage]; ok {
			sizes[pvcTypeQueue] = size
		}
	}

	return sizes
}

// deleteOrphanCachePVCs removes the cache PVCs of tiers that no longer request a
// dedicated cache volume. Only cache PVCs are reclaimed: they hold no backup
// data, and Kopia rebuilds the cache on demand.
func (r *ServerReconciler) deleteOrphanCachePVCs(ctx context.Context, server *kliov1alpha1.Server) error {
	contextLogger := logf.FromContext(ctx)

	orphaned := map[string]bool{
		pvcTypeCacheTier1: server.Spec.Tier1 == nil || server.Spec.Tier1.Cache == nil,
		pvcTypeCacheTier2: server.Spec.Tier2 == nil || server.Spec.Tier2.Cache == nil,
	}

	var pvcList corev1.PersistentVolumeClaimList
	if err := r.List(ctx, &pvcList,
		client.InNamespace(server.Namespace),
		client.MatchingLabels{klioServerLabel: server.Name},
	); err != nil {
		return fmt.Errorf("failed to list PVCs: %w", err)
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if !orphaned[pvc.Labels[pvcTypeLabel]] {
			continue
		}

		contextLogger.Info("Deleting cache PVC of a tier that no longer requests one", "pvc", pvc.Name)

		if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete orphan cache PVC %s: %w", pvc.Name, err)
		}

		r.Recorder.Eventf(server, nil, corev1.EventTypeNormal, "CachePVCDeleted",
			"DeleteOrphanCachePVC", "Cache PVC %s deleted: the tier has no dedicated cache volume", pvc.Name)
	}

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
