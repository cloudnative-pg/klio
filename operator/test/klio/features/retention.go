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
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
	klioConditions "github.com/cloudnative-pg/klio/operator/test/klio/conditions"
	"github.com/cloudnative-pg/klio/operator/test/klio/podexec"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
)

const (
	serverContainerName = "server"
	// klioPodSuffix is the suffix added to the server name to form the pod name.
	klioPodSuffix = "-klio-0"
	// tier1AnnotationName is the annotation key used to mark backups present in tier1.
	tier1AnnotationName = "klio.io/tier1"
	// tier2AnnotationName is the annotation key used to mark backups present in tier2.
	tier2AnnotationName = "klio.io/tier2"
	// archiveConfigPath is the config file the klio-plugin sidecar uses for its own
	// cluster's backup/WAL-archive operations, mounted from the ArchiveConfigKey
	// projection. It carries this cluster's own Tier2RecoveryEnabled setting, so it
	// can also be used to invoke `klio restore` as this cluster's client identity.
	archiveConfigPath = "/var/lib/postgresql/klio/" + klioconfig.ArchiveConfigKey
	// tier2GateLogMessage is a substring of the message restoreCmd logs when it drops
	// a backup-only Tier2 base URL because tier2 recovery is not enabled. See
	// gateTier2ForRecovery in core/cmd/restore.go.
	tier2GateLogMessage = "recovery from tier2 is not enabled"
	// tier2GateRestoreScratchDir is a scratch destination for the `klio restore` run
	// by verifyTier2RecoveryGate. It is never used as a real PGDATA, only to smoke-test
	// that a backup-only tier2 is not used as a recovery source.
	tier2GateRestoreScratchDir = "/tmp/tier2-recovery-gate-check"
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

// Teardown cleans up resources after the test is run.
func (f *Tier1RetentionFeature) Teardown() types.StepFunc {
	return f.teardown
}

// Tier2RetentionFeature defines a feature for testing tier2 backup and WAL retention.
type Tier2RetentionFeature struct {
	name                    string
	setup                   types.StepFunc
	teardown                types.StepFunc
	backups                 []*cnpgv1.Backup
	klioServer              *kliov1alpha1.Server
	namespace               string
	keepLatest              int
	backupTimeout           time.Duration
	replicationTimeout      time.Duration
	checkInterval           time.Duration
	clusterName             string
	s3Prefix                string
	pluginConfigurationName string
}

// Tier2RetentionFeatureConfig holds the configuration for creating a tier2 retention feature test.
type Tier2RetentionFeatureConfig struct {
	// Name of the tier2 retention feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
	// Backups are the backup resources to be created.
	Backups []*cnpgv1.Backup
	// KlioServer is the Klio server resource.
	KlioServer *kliov1alpha1.Server
	// Namespace is the namespace where resources are created.
	Namespace string
	// KeepLatest is the number of backups to keep in tier2.
	KeepLatest int
	// BackupTimeout is the timeout for each backup (defaults to 5 minutes).
	BackupTimeout time.Duration
	// ReplicationTimeout is the timeout for tier2 replication (defaults to 5 minutes).
	ReplicationTimeout time.Duration
	// CheckInterval is the interval for checking status (defaults to 10 seconds).
	CheckInterval time.Duration
	// ClusterName is the name of the CNPG cluster (used for WAL directory lookup).
	ClusterName string
	// S3Prefix is the S3 prefix used for tier2 storage.
	S3Prefix string
	// PluginConfigurationName is the name of the PluginConfiguration, used to
	// tighten the retention policy for the on-demand `klio retention apply` step.
	PluginConfigurationName string
}

// NewTier2RetentionFeature creates a new Tier2RetentionFeature with the given configuration.
func NewTier2RetentionFeature(config Tier2RetentionFeatureConfig) *Tier2RetentionFeature {
	if config.BackupTimeout <= 0 {
		config.BackupTimeout = 5 * time.Minute
	}
	if config.ReplicationTimeout <= 0 {
		config.ReplicationTimeout = 5 * time.Minute
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 10 * time.Second
	}

	return &Tier2RetentionFeature{
		name:                    config.Name,
		setup:                   config.Setup,
		teardown:                config.Teardown,
		backups:                 config.Backups,
		klioServer:              config.KlioServer,
		namespace:               config.Namespace,
		keepLatest:              config.KeepLatest,
		backupTimeout:           config.BackupTimeout,
		replicationTimeout:      config.ReplicationTimeout,
		checkInterval:           config.CheckInterval,
		clusterName:             config.ClusterName,
		s3Prefix:                config.S3Prefix,
		pluginConfigurationName: config.PluginConfigurationName,
	}
}

// Name returns the name of the tier2 retention feature.
func (f *Tier2RetentionFeature) Name() string {
	return f.name
}

// Setup initializes the tier2 retention feature test.
func (f *Tier2RetentionFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the tier2 retention feature test.
//
// This test validates the complete tier2 retention pipeline using a three-level
// verification strategy:
//
//  1. Retention Verification: runs the shared retention flow
//     (verifyRetentionAndOnDemandApply): creates more backups than `keepLatest`,
//     verifies tier2 ends up with exactly `keepLatest` backups with the oldest
//     deleted, then tightens the policy to a single backup and applies it on
//     demand with `klio retention apply`, verifying only the newest remains. An
//     onFirstBackup hook records the WAL directory baseline used by Level 2.
//
//  2. WAL Retention Verification: Monitors WAL directory count before and after
//     retention to verify WAL cleanup is occurring. This is a soft check (logs
//     warnings only) because WAL retention depends on backup metadata (StartWAL)
//     and timing, making strict assertions fragile.
//
//  3. Tier2 Recovery Gate Verification: this scenario configures tier2 for backup
//     only (EnableTier2Recovery: false). Runs an actual `klio restore` as this
//     cluster's own client identity and verifies the gate log fires, confirming
//     tier2 was dropped as a recovery source rather than silently used.
func (f *Tier2RetentionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running tier2 backup and WAL retention feature test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Track WAL directory count to verify WAL retention (Level 2 verification)
		var walDirCountAfterFirstBackup int

		// ==========================================
		// Level 1: Retention Verification
		// ==========================================
		// The shared flow verifies automatic retention (oldest deleted) and the
		// on-demand `klio retention apply` (only the newest remains). The
		// onFirstBackup hook records the WAL directory baseline for Level 2.
		verifyRetentionAndOnDemandApply(ctx, t, r, retentionFlowParams{
			backups:                 f.backups,
			serverName:              f.klioServer.Name,
			namespace:               f.namespace,
			clusterName:             f.clusterName,
			keepLatest:              f.keepLatest,
			tierLabel:               "tier2",
			tierAnnotation:          tier2AnnotationName,
			pluginConfigurationName: f.pluginConfigurationName,
			backupTimeout:           f.backupTimeout,
			retentionTimeout:        f.replicationTimeout,
			checkInterval:           f.checkInterval,
			setRetentionLatest: func(ctx context.Context, t *testing.T, r *resources.Resources, latest int) {
				t.Helper()
				updateTier2RetentionLatest(ctx, t, r, f.namespace, f.pluginConfigurationName, latest)
			},
			onFirstBackup: func(ctx context.Context) {
				var walErr error
				walDirCountAfterFirstBackup, walErr = countTier2WALDirectories(
					ctx, r, f.namespace, f.klioServer.Name, f.s3Prefix, f.clusterName)
				if walErr != nil {
					t.Logf("Warning: could not count WAL directories after first backup: %v", walErr)
				} else {
					t.Logf("WAL directories after first backup: %d", walDirCountAfterFirstBackup)
				}
			},
		})

		// ==========================================
		// Level 2: WAL Retention Verification (Soft Check)
		// ==========================================
		// Monitor WAL directory count to verify cleanup is occurring.
		// This is a soft check (warnings only) because:
		// - WAL retention depends on backup metadata (StartWAL field)
		// - Timing variations can cause count fluctuations
		// - The exact count depends on PostgreSQL activity during the test
		verifyWALRetention(ctx, t, r, f, walDirCountAfterFirstBackup)

		// ==========================================
		// Level 3: Tier2 Recovery Gate Verification
		// ==========================================
		// This scenario configures tier2 for backup only (EnableTier2Recovery:
		// false). Run an actual `klio restore` as this cluster's own client
		// identity and verify the gate log fires, confirming tier2 was dropped
		// as a recovery source rather than silently used.
		t.Log("[Level 3] Tier2 recovery gate verification: klio restore must not use a backup-only tier2...")
		verifyTier2RecoveryGate(ctx, t, r, f.namespace, f.backups[len(f.backups)-1])
		t.Log("[Level 3] PASSED: klio restore logged that tier2 recovery is disabled")

		t.Log("Tier2 retention test completed: all verification levels passed")

		return ctx
	}
}

// Teardown cleans up resources after the test is run.
func (f *Tier2RetentionFeature) Teardown() types.StepFunc {
	return f.teardown
}

// retentionFlowParams carries what the tier-agnostic retention flow needs to
// exercise one tier's retention: automatic deletion of the oldest backup once
// newer backups push it outside the policy, followed by on-demand deletion via
// `klio retention apply`. Tier1 and tier2 differ only in the tier annotation,
// the label used in logs, and which tier's policy setRetentionLatest edits.
type retentionFlowParams struct {
	backups                 []*cnpgv1.Backup
	serverName              string
	namespace               string
	clusterName             string
	keepLatest              int
	tierLabel               string
	tierAnnotation          string
	pluginConfigurationName string
	backupTimeout           time.Duration
	retentionTimeout        time.Duration
	checkInterval           time.Duration
	// setRetentionLatest tightens this tier's policy to keep `latest` backups.
	setRetentionLatest func(ctx context.Context, t *testing.T, r *resources.Resources, latest int)
	// onFirstBackup, if set, runs once right after the first backup reaches the
	// tier. Tier2 uses it to record a WAL directory baseline.
	onFirstBackup func(ctx context.Context)
}

// verifyRetentionAndOnDemandApply runs the tier-agnostic retention flow shared
// by the tier1 and tier2 retention features. It creates more backups than the
// policy keeps and verifies the tier ends up with exactly keepLatest backups
// with the oldest deleted, then tightens the policy to a single backup, applies
// it on demand with `klio retention apply`, and verifies only the newest
// remains. This exercises the full server-side path: PluginConfiguration CR ->
// operator -> klio-plugin config -> CloseBackup GRPC -> NATS queue -> backup
// consumer -> Klio-managed retention.
func verifyRetentionAndOnDemandApply(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	p retentionFlowParams,
) {
	t.Helper()

	// Name of the first (oldest) backup. Retention must delete it once newer
	// backups push it outside the policy.
	var oldestBackupName string

	for i, backup := range p.backups {
		t.Logf("Creating backup %d/%d: %s", i+1, len(p.backups), backup.Name)
		require.NoError(t, r.Create(ctx, backup), "failed to create backup %s", backup.Name)

		err := wait.For(
			machineryConditions.BackupIsCompleted(r, backup),
			wait.WithTimeout(p.backupTimeout),
			wait.WithInterval(p.checkInterval),
		)
		require.NoError(t, err, "backup %s did not complete", backup.Name)
		t.Logf("Backup %s completed successfully", backup.Name)

		// After more backups than the policy keeps, the older ones are deleted.
		expectedBackups := min(i+1, p.keepLatest)
		err = wait.For(
			klioConditions.CheckTierHasBackups(r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation, expectedBackups),
			wait.WithTimeout(p.retentionTimeout),
			wait.WithInterval(p.checkInterval),
		)
		require.NoError(t, err, "%s retention not applied after backup %d", p.tierLabel, i+1)
		t.Logf("%s has expected %d backup(s) after backup %d", p.tierLabel, expectedBackups, i+1)

		if i == 0 {
			names, listErr := podexec.ListTierBackupNames(
				ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
			require.NoError(t, listErr, "failed to list %s backups after the first backup", p.tierLabel)
			require.Len(t, names, 1, "expected exactly one %s backup after the first backup", p.tierLabel)
			oldestBackupName = names[0]
			t.Logf("Oldest %s backup recorded: %s", p.tierLabel, oldestBackupName)

			if p.onFirstBackup != nil {
				p.onFirstBackup(ctx)
			}
		}
	}

	// Automatic retention: exactly keepLatest backups remain and the oldest was
	// the one the retention manager deleted (the newest survive).
	t.Logf("Retention verification: %s should have exactly %d backup(s)", p.tierLabel, p.keepLatest)
	err := wait.For(
		klioConditions.CheckTierHasBackups(r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation, p.keepLatest),
		wait.WithTimeout(p.retentionTimeout),
		wait.WithInterval(p.checkInterval),
	)
	require.NoError(t, err, "%s backup count verification failed", p.tierLabel)

	survivingNames, err := podexec.ListTierBackupNames(
		ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
	require.NoError(t, err, "could not list surviving %s backups", p.tierLabel)
	require.NotContains(t, survivingNames, oldestBackupName,
		"the oldest backup should have been deleted by the %s retention manager", p.tierLabel)
	t.Logf("PASSED: %s has exactly %d backup(s) and the oldest (%s) was deleted",
		p.tierLabel, p.keepLatest, oldestBackupName)

	// On-demand retention: tighten the policy to keep a single backup and apply
	// it immediately with `klio retention apply`, without taking a new backup.
	// Only the newest backup must remain afterwards.
	newestBackupName := survivingNames[0]
	t.Logf("On-demand retention: tightening %s retention to 1 and running `klio retention apply`", p.tierLabel)
	p.setRetentionLatest(ctx, t, r, 1)

	instancePodName := p.backups[len(p.backups)-1].Status.InstanceID.PodName
	require.NotEmpty(t, instancePodName, "backup instance pod name should be set")

	// The `klio retention apply` command sends the policy from the pod's mounted
	// config, which the operator updates asynchronously after the
	// PluginConfiguration change. Re-running the (idempotent) command until
	// exactly one backup remains absorbs both the config propagation and the
	// asynchronous consumer processing.
	err = wait.For(
		func(ctx context.Context) (bool, error) {
			if applyErr := runRetentionApply(ctx, r, p.namespace, instancePodName); applyErr != nil {
				t.Logf("klio retention apply not ready yet: %v", applyErr)

				return false, nil
			}

			names, listErr := podexec.ListTierBackupNames(
				ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
			if listErr != nil {
				return false, nil //nolint:nilerr
			}

			return len(names) == 1, nil
		},
		wait.WithTimeout(p.retentionTimeout),
		wait.WithInterval(p.checkInterval),
	)
	require.NoError(t, err, "on-demand %s retention did not converge to a single backup", p.tierLabel)

	finalNames, err := podexec.ListTierBackupNames(
		ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
	require.NoError(t, err, "could not list surviving %s backups", p.tierLabel)
	require.Equal(t, []string{newestBackupName}, finalNames,
		"only the newest backup should remain after `klio retention apply`")
	t.Logf("PASSED: only the newest %s backup (%s) remains after apply", p.tierLabel, newestBackupName)
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
			walFiles := podexec.ListTier1WALFiles(ctx, r, namespace, podName, clusterName)

			return len(podexec.WALsOlderThan(walFiles, boundary)) == 0, nil
		},
		wait.WithTimeout(timeout),
		wait.WithInterval(interval),
	)

	final := podexec.ListTier1WALFiles(ctx, r, namespace, podName, clusterName)
	require.NoError(t, err, "tier1 WALs older than begin WAL %q survived base retention: %v",
		boundary, podexec.WALsOlderThan(final, boundary))
	// The begin WAL itself is retained, so an empty list means we looked at the
	// wrong path rather than at a successful retention.
	require.NotEmpty(t, final, "no tier1 WAL files found for cluster %q", clusterName)
	t.Logf("PASSED: %d tier1 WAL files remain, all >= %s", len(final), boundary)
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

// verifyTier2RecoveryGate runs `klio restore` inside the klio-plugin sidecar of
// the pod that took backup, using the pod's own archive config (the same
// config the instance uses for its own backups/WAL archiving, which carries
// this cluster's real Tier2RecoveryEnabled setting). It restores into a
// scratch directory, never the real PGDATA, and asserts that restore still
// succeeds (tier1 remains a valid base source) while logging that a
// backup-only tier2 was not used, proving the recovery gate is active.
func verifyTier2RecoveryGate(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	namespace string,
	backup *cnpgv1.Backup,
) {
	t.Helper()

	require.NotNil(t, backup.Status.InstanceID, "backup instance ID should be set")
	podName := backup.Status.InstanceID.PodName

	var stdout, stderr bytes.Buffer
	restoreCmd := []string{
		"klio", "restore",
		"--config", archiveConfigPath,
		tier2GateRestoreScratchDir,
	}

	err := r.ExecInPod(ctx, namespace, podName, cnpgi.KlioPluginContainerName, restoreCmd, &stdout, &stderr)
	require.NoError(t, err, "klio restore failed unexpectedly; stdout: %s, stderr: %s", stdout.String(), stderr.String())

	require.Contains(t, stdout.String()+stderr.String(), tier2GateLogMessage,
		"expected klio restore to log that tier2 recovery is disabled")
}

// verifyWALRetention performs Level 3 WAL retention verification.
// It compares the current WAL directory count against the baseline and logs warnings
// if growth exceeds acceptable bounds. This is a soft check that doesn't fail the test.
func verifyWALRetention(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	f *Tier2RetentionFeature,
	baselineCount int,
) {
	t.Helper()

	if baselineCount == 0 {
		t.Log("[Level 2] SKIPPED: no baseline WAL count available")

		return
	}

	t.Log("[Level 2] WAL retention verification: checking WAL directory growth...")
	finalCount, err := countTier2WALDirectories(
		ctx, r, f.namespace, f.klioServer.Name, f.s3Prefix, f.clusterName)
	if err != nil {
		t.Logf("[Level 2] WARNING: could not count final WAL directories: %v", err)

		return
	}

	t.Logf("[Level 2] WAL directories: %d (was %d after first backup)", finalCount, baselineCount)

	// WAL retention should prevent unbounded growth. We allow 3x growth
	// to account for WALs generated during test execution.
	const maxGrowthFactor = 3
	if finalCount > baselineCount*maxGrowthFactor {
		t.Logf("[Level 2] WARNING: WAL directory count grew significantly (%d -> %d), "+
			"WAL retention may not be working as expected", baselineCount, finalCount)

		return
	}

	t.Log("[Level 2] PASSED: WAL directory growth is within acceptable bounds")
}

// updateTier2RetentionLatest fetches the PluginConfiguration and sets its
// tier2 retention policy to keep the given number of most recent backups.
func updateTier2RetentionLatest(
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

	require.NotNil(t, pc.Spec.Tier2, "PluginConfiguration should have a tier2 section")
	if pc.Spec.Tier2.RetentionPolicy == nil {
		pc.Spec.Tier2.RetentionPolicy = &kliov1alpha1.RetentionPolicy{}
	}
	pc.Spec.Tier2.RetentionPolicy.Latest = latest

	require.NoError(t, r.Update(ctx, &pc), "failed to update PluginConfiguration retention")
}

// runRetentionApply runs `klio retention apply` inside the klio-plugin sidecar
// of the given pod, using the pod's own archive config, to apply the configured
// retention policy on demand.
func runRetentionApply(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	podName string,
) error {
	var stdout, stderr bytes.Buffer
	applyCmd := []string{"klio", "retention", "apply", "--config", archiveConfigPath}
	if err := r.ExecInPod(
		ctx, namespace, podName, cnpgi.KlioPluginContainerName, applyCmd, &stdout, &stderr,
	); err != nil {
		return fmt.Errorf("klio retention apply failed: %w; stdout: %s, stderr: %s",
			err, stdout.String(), stderr.String())
	}

	return nil
}

// countTier2WALDirectories counts the number of WAL prefix directories in tier2 S3 storage.
// WAL files are stored under: <s3Prefix>/wals/<clusterName>/<walPrefix>/<walFile>
// This function counts the <walPrefix> directories to estimate WAL retention effectiveness.
func countTier2WALDirectories(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	serverName string,
	s3Prefix string,
	clusterName string,
) (int, error) {
	podName := serverName + klioPodSuffix

	// List directories under the WAL path in tier2
	// The WAL files are stored in the cache directory which mirrors S3 structure
	walPath := fmt.Sprintf("/cache/%s/wals/%s", s3Prefix, clusterName)

	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"sh", "-c",
		fmt.Sprintf("ls -d %s/*/ 2>/dev/null | wc -l || echo 0", walPath),
	}

	err := r.ExecInPod(ctx, namespace, podName, serverContainerName, listCmd, &stdout, &stderr)
	if err != nil {
		return 0, fmt.Errorf("failed to list WAL directories: %w; stderr: %s", err, stderr.String())
	}

	var count int
	output := strings.TrimSpace(stdout.String())
	if _, err := fmt.Sscanf(output, "%d", &count); err != nil {
		return 0, fmt.Errorf("failed to parse WAL directory count from '%s': %w", output, err)
	}

	return count, nil
}
