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

package e2e

import (
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	klioFeatures "github.com/cloudnative-pg/klio/operator/test/klio/features"
	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/namespaces"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
)

type missingPluginConfigurationScenario struct {
	pluginTestResources

	name string
}

func (c *missingPluginConfigurationScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for missing PluginConfiguration feature: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Create all resources EXCEPT PluginConfiguration
	createNamespace(ctx, t, r, c.namespace)
	require.NoError(t, r.Create(ctx, c.issuer), "failed to create issuer")
	require.NoError(t, r.Create(ctx, c.caCertificate), "failed to create CA certificate")
	require.NoError(t, r.Create(ctx, c.caIssuer), "failed to create CA issuer")
	require.NoError(t, r.Create(ctx, c.certificate), "failed to create certificate")
	require.NoError(t, r.Create(ctx, c.userCertificate), "failed to create user certificate")
	require.NoError(t, r.Create(ctx, c.encryptionSecret), "failed to create encryption secret")
	require.NoError(t, r.Create(ctx, c.identitySecret), "failed to create identity secret")
	require.NoError(t, r.Create(ctx, c.klioServer), "failed to create Klio server")

	t.Log("Waiting for Klio server to be ready")
	err = wait.For(
		conditions.KlioServerIsReady(r, c.klioServer),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "failed to wait for Klio server to be ready")

	// Create cluster WITHOUT PluginConfiguration - this triggers requeue behavior
	t.Log("Creating CNPG cluster without PluginConfiguration (should trigger requeue)")
	require.NoError(t, r.Create(ctx, c.cnpgCluster), "failed to create CNPG Cluster")

	t.Logf("Setup complete for missing PluginConfiguration feature: %s", c.name)

	return ctx
}

func (c *missingPluginConfigurationScenario) Run(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Running missing PluginConfiguration test: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Step 1: Wait briefly and verify cluster is NOT in unrecoverable state
	t.Log("Waiting 20s to verify cluster doesn't become unrecoverable")
	time.Sleep(20 * time.Second)

	var cluster cnpgv1.Cluster
	require.NoError(t,
		r.Get(ctx, c.cnpgCluster.Name, c.namespace.Name, &cluster),
		"failed to get cluster")

	// Verify cluster is NOT unrecoverable or in plugin failure state
	t.Logf("Cluster phase: %s", cluster.Status.Phase)
	require.NotEqual(t, cnpgv1.PhaseUnrecoverable, cluster.Status.Phase,
		"cluster should NOT be in unrecoverable state while waiting for PluginConfiguration")
	require.NotEqual(t, cnpgv1.PhaseFailurePlugin, cluster.Status.Phase,
		"cluster should NOT be in plugin failure state while waiting for PluginConfiguration")

	// Verify no instances are ready yet (cluster is waiting for PluginConfiguration)
	require.Equal(t, 0, cluster.Status.ReadyInstances,
		"no instances should be ready while PluginConfiguration is missing")

	// Step 2: Create PluginConfiguration
	t.Log("Creating PluginConfiguration - cluster should now proceed")
	require.NoError(t, r.Create(ctx, c.klioPluginConfiguration),
		"failed to create Klio plugin configuration")

	// Step 3: Wait for cluster to become healthy
	t.Log("Waiting for cluster to become healthy")
	err = wait.For(
		machineryConditions.ClusterIsReady(r, c.cnpgCluster),
		wait.WithTimeout(5*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "cluster should become healthy after PluginConfiguration is created")

	// Verify final state
	require.NoError(t,
		r.Get(ctx, c.cnpgCluster.Name, c.namespace.Name, &cluster),
		"failed to get final cluster state")

	t.Logf("Final cluster state: Phase=%s, ReadyInstances=%d/%d",
		cluster.Status.Phase,
		cluster.Status.ReadyInstances,
		cluster.Spec.Instances)

	require.Equal(t, cluster.Spec.Instances, cluster.Status.ReadyInstances,
		"all instances should be ready")

	t.Log("Missing PluginConfiguration test completed successfully")

	return ctx
}

func (c *missingPluginConfigurationScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for missing PluginConfiguration feature: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	namespaces.DumpNamespaceOnFailure(ctx, t, r, testCfg.LogDir, c.namespace.Name, testconfig.DumpedKinds())
	require.NoError(t, r.Delete(ctx, c.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for missing PluginConfiguration feature: %s", c.name)

	return ctx
}

// MissingPluginConfiguration creates a feature that tests cluster behavior when
// PluginConfiguration is missing. It verifies that:
// 1. Cluster doesn't become unrecoverable when PluginConfiguration is missing
// 2. Cluster proceeds to healthy state after PluginConfiguration is created.
func MissingPluginConfiguration(namespace string) *klioFeatures.SimpleFeature {
	scenario := &missingPluginConfigurationScenario{
		pluginTestResources: newPluginTestResources(namespace),
		name:                "MissingPluginConfiguration",
	}

	return klioFeatures.NewSimpleFeature(klioFeatures.SimpleFeatureConfig{
		Name:     "MissingPluginConfiguration",
		Setup:    scenario.Setup,
		Run:      scenario.Run,
		Teardown: scenario.Teardown,
	})
}
