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

package features

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8swait "k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

const (
	// klioServerLabelKey is the label key used to identify PVCs belonging to a Klio server.
	klioServerLabelKey = "klio.cnpg.io/klio-server"
	// pvcTypeLabelKey is the label key used to identify the type of PVC.
	pvcTypeLabelKey = "klio.cnpg.io/pvcType"
)

// PVCResizeFeature defines a feature for testing PVC resize functionality.
type PVCResizeFeature struct {
	name         string
	setup        types.StepFunc
	teardown     types.StepFunc
	klioServer   *kliov1alpha1.Server
	namespace    string
	newDataSize  resource.Quantity
	newCacheSize resource.Quantity
	newQueueSize resource.Quantity
	timeout      time.Duration
	interval     time.Duration
}

// PVCResizeFeatureConfig holds the configuration for creating a PVC resize feature test.
type PVCResizeFeatureConfig struct {
	// Name of the PVC resize feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
	// KlioServer is the Klio server resource.
	KlioServer *kliov1alpha1.Server
	// Namespace is the namespace where resources are created.
	Namespace string
	// NewDataSize is the new size for the data PVC.
	NewDataSize resource.Quantity
	// NewCacheSize is the new size for the cache PVC.
	NewCacheSize resource.Quantity
	// NewQueueSize is the new size for the queue PVC.
	NewQueueSize resource.Quantity
	// Timeout for waiting for PVC resize (defaults to 5 minutes).
	Timeout time.Duration
	// Interval for checking PVC resize status (defaults to 5 seconds).
	Interval time.Duration
}

// NewPVCResizeFeature creates a new PVCResizeFeature with the given configuration.
func NewPVCResizeFeature(config PVCResizeFeatureConfig) *PVCResizeFeature {
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Minute
	}
	if config.Interval <= 0 {
		config.Interval = 5 * time.Second
	}

	return &PVCResizeFeature{
		name:         config.Name,
		setup:        config.Setup,
		teardown:     config.Teardown,
		klioServer:   config.KlioServer,
		namespace:    config.Namespace,
		newDataSize:  config.NewDataSize,
		newCacheSize: config.NewCacheSize,
		newQueueSize: config.NewQueueSize,
		timeout:      config.Timeout,
		interval:     config.Interval,
	}
}

// Name returns the name of the PVC resize feature.
func (f *PVCResizeFeature) Name() string {
	return f.name
}

// Setup initializes the PVC resize feature test.
func (f *PVCResizeFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the PVC resize feature test.
func (f *PVCResizeFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running PVC resize feature test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Record initial PVC sizes
		initialSizes, err := getPVCSizes(ctx, r, f.namespace, f.klioServer.Name)
		require.NoError(t, err, "failed to get initial PVC sizes")
		t.Logf("Initial PVC sizes: %v", initialSizes)

		// Update the Server spec with new PVC sizes
		expectedSizes := f.updateServerPVCSizes(ctx, t, r)

		// Wait for PVCs to be resized
		t.Log("Waiting for PVCs to be resized...")
		err = wait.For(
			checkPVCsResized(r, f.namespace, f.klioServer.Name, expectedSizes),
			wait.WithTimeout(f.timeout),
			wait.WithInterval(f.interval),
		)
		require.NoError(t, err, "PVCs were not resized within timeout")

		// Verify final sizes
		finalSizes, err := getPVCSizes(ctx, r, f.namespace, f.klioServer.Name)
		require.NoError(t, err, "failed to get final PVC sizes")
		t.Logf("Final PVC sizes: %v", finalSizes)

		verifyPVCSizes(t, expectedSizes, initialSizes, finalSizes)

		t.Log("PVC resize test completed successfully")

		return ctx
	}
}

// Teardown cleans up resources after the test is run.
func (f *PVCResizeFeature) Teardown() types.StepFunc {
	return f.teardown
}

// updateServerPVCSizes updates the Server spec with new PVC sizes and returns the expected sizes.
func (f *PVCResizeFeature) updateServerPVCSizes(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
) map[string]resource.Quantity {
	t.Helper()
	t.Log("Updating Server spec with new PVC sizes...")

	var server kliov1alpha1.Server
	err := r.Get(ctx, f.klioServer.Name, f.namespace, &server)
	require.NoError(t, err, "failed to get Server")

	expectedSizes := make(map[string]resource.Quantity)

	if server.Spec.Tier1 != nil && !f.newDataSize.IsZero() {
		server.Spec.Tier1.Data.PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage] = f.newDataSize
		expectedSizes["data"] = f.newDataSize
		t.Logf("Updating data PVC size to %s", f.newDataSize.String())
	}

	if server.Spec.Tier1 != nil && !f.newCacheSize.IsZero() {
		server.Spec.Tier1.Cache.PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage] = f.newCacheSize
		expectedSizes["cachetier1"] = f.newCacheSize
		t.Logf("Updating cachetier1 PVC size to %s", f.newCacheSize.String())
	}

	if server.Spec.Queue != nil && !f.newQueueSize.IsZero() {
		server.Spec.Queue.PersistentVolumeClaimTemplate.Resources.Requests[corev1.ResourceStorage] = f.newQueueSize
		expectedSizes["queue"] = f.newQueueSize
		t.Logf("Updating queue PVC size to %s", f.newQueueSize.String())
	}

	err = r.Update(ctx, &server)
	require.NoError(t, err, "failed to update Server with new PVC sizes")

	return expectedSizes
}

// verifyPVCSizes verifies that PVCs have been resized to the expected sizes.
func verifyPVCSizes(
	t *testing.T,
	expectedSizes map[string]resource.Quantity,
	initialSizes map[string]resource.Quantity,
	finalSizes map[string]resource.Quantity,
) {
	t.Helper()

	for pvcType, expectedSize := range expectedSizes {
		actualSize, exists := finalSizes[pvcType]
		require.True(t, exists, "PVC type %s not found", pvcType)
		require.GreaterOrEqual(t, actualSize.Cmp(expectedSize), 0,
			"PVC %s size %s is less than expected %s",
			pvcType, (&actualSize).String(), (&expectedSize).String())
		initialSize := initialSizes[pvcType]
		t.Logf("PVC %s resized successfully: %s -> %s", pvcType, (&initialSize).String(), (&actualSize).String())
	}
}

// getPVCSizes returns a map of PVC type labels to their current sizes.
func getPVCSizes(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	serverName string,
) (map[string]resource.Quantity, error) {
	var pvcList corev1.PersistentVolumeClaimList
	err := r.List(ctx, &pvcList,
		resources.WithLabelSelector(klioServerLabelKey+"="+serverName),
	)
	if err != nil {
		return nil, err
	}

	sizes := make(map[string]resource.Quantity)
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if pvc.Namespace != namespace {
			continue
		}

		pvcType, exists := pvc.Labels[pvcTypeLabelKey]
		if !exists {
			continue
		}

		if size, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			sizes[pvcType] = size
		}
	}

	return sizes, nil
}

// checkPVCsResized checks if PVCs have been resized to the expected sizes.
// It checks the PVC spec (requested size) rather than status (actual size)
// because the actual resize may take time and depends on the storage provider.
func checkPVCsResized(
	r *resources.Resources,
	namespace string,
	serverName string,
	expectedSizes map[string]resource.Quantity,
) k8swait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		sizes, err := getPVCSizes(ctx, r, namespace, serverName)
		if err != nil {
			return false, nil //nolint:nilerr
		}

		for pvcType, expectedSize := range expectedSizes {
			actualSize, exists := sizes[pvcType]
			if !exists {
				return false, nil
			}

			// Check if the requested size matches or exceeds expected
			if actualSize.Cmp(expectedSize) < 0 {
				return false, nil
			}
		}

		return true, nil
	}
}
