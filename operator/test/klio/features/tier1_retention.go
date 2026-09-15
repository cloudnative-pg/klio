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

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// Tier1RetentionFeature defines a feature for testing tier1 backup retention on
// a tier1-only deployment.
type Tier1RetentionFeature struct {
	name                    string
	setup                   types.StepFunc
	teardown                types.StepFunc
	backups                 []*cnpgv1.Backup
	klioServer              *kliov1alpha1.Server
	namespace               string
	keepLatest              int
	backupTimeout           time.Duration
	checkInterval           time.Duration
	clusterName             string
	pluginConfigurationName string
}

// Tier1RetentionFeatureConfig holds the configuration for creating a tier1
// retention feature test.
type Tier1RetentionFeatureConfig struct {
	// Name of the tier1 retention feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
	// Backups are the backup resources to be created, in order.
	Backups []*cnpgv1.Backup
	// KlioServer is the Klio server resource.
	KlioServer *kliov1alpha1.Server
	// Namespace is the namespace where resources are created.
	Namespace string
	// KeepLatest is the number of backups the tier1 policy keeps.
	KeepLatest int
	// ClusterName is the name of the CNPG cluster whose backups are counted.
	ClusterName string
	// PluginConfigurationName is the name of the PluginConfiguration, used to
	// tighten the retention policy for the on-demand `klio retention apply` step.
	PluginConfigurationName string
	// BackupTimeout is the timeout for each backup and its retention (defaults
	// to 5 minutes).
	BackupTimeout time.Duration
	// CheckInterval is the interval for polling status (defaults to 10 seconds).
	CheckInterval time.Duration
}

// NewTier1RetentionFeature creates a new Tier1RetentionFeature with the given
// configuration.
func NewTier1RetentionFeature(config Tier1RetentionFeatureConfig) *Tier1RetentionFeature {
	if config.BackupTimeout <= 0 {
		config.BackupTimeout = 5 * time.Minute
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 10 * time.Second
	}

	return &Tier1RetentionFeature{
		name:                    config.Name,
		setup:                   config.Setup,
		teardown:                config.Teardown,
		backups:                 config.Backups,
		klioServer:              config.KlioServer,
		namespace:               config.Namespace,
		keepLatest:              config.KeepLatest,
		backupTimeout:           config.BackupTimeout,
		checkInterval:           config.CheckInterval,
		clusterName:             config.ClusterName,
		pluginConfigurationName: config.PluginConfigurationName,
	}
}

// Name returns the name of the tier1 retention feature.
func (f *Tier1RetentionFeature) Name() string {
	return f.name
}

// Setup initializes the tier1 retention feature test.
func (f *Tier1RetentionFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the tier1 retention feature test. It creates more backups than
// the policy keeps and verifies that tier1 ends up with exactly `keepLatest`
// backups and that the oldest one was the backup the retention manager deleted
// (the newest survive). This exercises the full server-side tier1 path:
// PluginConfiguration CR -> operator -> klio-plugin config -> CloseBackup GRPC
// -> NATS queue -> backup consumer -> Klio-managed tier1 retention. It then
// tightens the policy to a single backup and applies it on demand with
// `klio retention apply`, verifying only the newest remains.
func (f *Tier1RetentionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running tier1 backup retention feature test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		verifyRetentionAndOnDemandApply(ctx, t, r, retentionFlowParams{
			backups:                 f.backups,
			serverName:              f.klioServer.Name,
			namespace:               f.namespace,
			clusterName:             f.clusterName,
			keepLatest:              f.keepLatest,
			tierLabel:               "tier1",
			tierAnnotation:          tier1AnnotationName,
			pluginConfigurationName: f.pluginConfigurationName,
			backupTimeout:           f.backupTimeout,
			retentionTimeout:        f.backupTimeout,
			checkInterval:           f.checkInterval,
			setRetentionLatest: func(ctx context.Context, t *testing.T, r *resources.Resources, latest int) {
				t.Helper()
				updateTier1RetentionLatest(ctx, t, r, f.namespace, f.pluginConfigurationName, latest)
			},
		})

		// Base retention moved the WAL horizon: only the newest backup is left,
		// so every tier1 WAL older than its begin WAL must be gone too.
		verifyTier1WALHorizon(ctx, t, r, f.namespace, f.klioServer.Name, f.clusterName,
			f.backups[len(f.backups)-1], f.backupTimeout, f.checkInterval)

		return ctx
	}
}

// verifyTier1WALHorizon waits until no tier1 WAL segment older than the begin
// WAL of the given backup survives, which proves the WAL retention horizon was
// recomputed from the catalog after base retention deleted the older backups.
func verifyTier1WALHorizon(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	namespace string,
	serverName string,
	clusterName string,
	backup *cnpgv1.Backup,
	timeout time.Duration,
	interval time.Duration,
) {
	t.Helper()

	var newest cnpgv1.Backup
	require.NoError(t, r.Get(ctx, backup.Name, namespace, &newest), "failed to refresh backup %s", backup.Name)
	boundary := newest.Status.BeginWal
	require.NotEmpty(t, boundary, "completed backup %s has no begin WAL in its status", backup.Name)

	podName := serverName + klioPodSuffix
	t.Logf("WAL horizon: waiting for tier1 WALs older than %s to be removed", boundary)
	err := wait.For(
		func(ctx context.Context) (bool, error) {
			walFiles := ListTier1WALFiles(ctx, r, namespace, podName, clusterName)

			return len(WALsOlderThan(walFiles, boundary)) == 0, nil
		},
		wait.WithTimeout(timeout),
		wait.WithInterval(interval),
	)

	final := ListTier1WALFiles(ctx, r, namespace, podName, clusterName)
	require.NoError(t, err, "tier1 WALs older than begin WAL %q survived base retention: %v",
		boundary, WALsOlderThan(final, boundary))
	// The begin WAL itself is retained, so an empty list means we looked at the
	// wrong path rather than at a successful retention.
	require.NotEmpty(t, final, "no tier1 WAL files found for cluster %q", clusterName)
	t.Logf("PASSED: %d tier1 WAL files remain, all >= %s", len(final), boundary)
}

// Teardown cleans up resources after the test is run.
func (f *Tier1RetentionFeature) Teardown() types.StepFunc {
	return f.teardown
}

// updateTier1RetentionLatest fetches the PluginConfiguration and sets its
// tier1 retention policy to keep the given number of most recent backups.
func updateTier1RetentionLatest(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	namespace string,
	pluginConfigurationName string,
	latest int,
) {
	t.Helper()

	var pc kliov1alpha1.PluginConfiguration
	require.NoError(t, r.Get(ctx, pluginConfigurationName, namespace, &pc),
		"failed to get PluginConfiguration %q", pluginConfigurationName)

	require.NotNil(t, pc.Spec.Tier1, "PluginConfiguration should have a tier1 section")
	if pc.Spec.Tier1.RetentionPolicy == nil {
		pc.Spec.Tier1.RetentionPolicy = &kliov1alpha1.RetentionPolicy{}
	}
	pc.Spec.Tier1.RetentionPolicy.Latest = latest

	require.NoError(t, r.Update(ctx, &pc), "failed to update PluginConfiguration retention")
}
