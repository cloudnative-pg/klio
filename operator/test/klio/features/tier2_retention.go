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
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	k8swait "k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
)

const (
	serverContainerName = "server"
	// klioPodSuffix is the suffix added to the server name to form the pod name.
	klioPodSuffix = "-klio-0"
	// tier2AnnotationName is the annotation key used to mark backups present in tier2.
	tier2AnnotationName = "klio.io/tier2"
	// presentAnnotationValue is the value set when a backup is present in a tier.
	presentAnnotationValue = "present"
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
//  1. Retention Verification: creates more backups than `keepLatest` and verifies
//     that tier2 ends up with exactly `keepLatest` backups and that the oldest one
//     was the backup the retention manager deleted (the newest survive). This
//     exercises the full path: PluginConfiguration CR -> operator -> klio-plugin
//     config -> CloseBackup GRPC -> NATS queue -> backup consumer -> Klio-managed
//     retention. It then tightens the policy to a single backup and applies it on
//     demand with `klio retention apply`, verifying only the newest remains.
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

		// Name of the first (oldest) backup that reached tier2. Retention must
		// eventually delete it once newer backups push it outside the policy.
		var oldestTier2BackupName string

		// Create more backups than keepLatest to trigger retention
		for i, backup := range f.backups {
			t.Logf("Creating backup %d/%d: %s", i+1, len(f.backups), backup.Name)
			require.NoError(t, r.Create(ctx, backup), "failed to create backup %s", backup.Name)

			// Wait for backup to complete
			err = wait.For(
				machineryConditions.BackupIsCompleted(r, backup),
				wait.WithTimeout(f.backupTimeout),
				wait.WithInterval(f.checkInterval),
			)
			require.NoError(t, err, "backup %s did not complete", backup.Name)
			t.Logf("Backup %s completed successfully", backup.Name)

			// Wait for backup to be replicated to tier2
			t.Logf("Waiting for backup %d to reach tier2...", i+1)

			// If we've taken more backups than retention allows, we expect the older ones to be deleted
			expectedBackups := min(i+1, f.keepLatest)

			err = wait.For(
				checkTier2HasBackups(r, f.namespace, f.klioServer.Name, expectedBackups),
				wait.WithTimeout(f.replicationTimeout),
				wait.WithInterval(f.checkInterval),
			)
			require.NoError(t, err, "tier2 replication/retention not completed for backup %d", i+1)
			t.Logf("Tier2 has expected %d backup(s) after backup %d", expectedBackups, i+1)

			// After the first backup, record the WAL directory count as baseline
			// and remember which backup reached tier2 first (the oldest one).
			if i == 0 {
				names, listErr := listTier2BackupNames(ctx, r, f.namespace, f.klioServer.Name)
				require.NoError(t, listErr, "failed to list tier2 backups after the first backup")
				require.Len(t, names, 1, "expected exactly one tier2 backup after the first backup")
				oldestTier2BackupName = names[0]
				t.Logf("Oldest tier2 backup recorded: %s", oldestTier2BackupName)

				walDirCountAfterFirstBackup, err = countTier2WALDirectories(
					ctx, r, f.namespace, f.klioServer.Name, f.s3Prefix, f.clusterName)
				if err != nil {
					t.Logf("Warning: could not count WAL directories after first backup: %v", err)
				} else {
					t.Logf("WAL directories after first backup: %d", walDirCountAfterFirstBackup)
				}
			}
		}

		// ==========================================
		// Level 1: Retention Verification
		// ==========================================
		// Verify that tier2 contains exactly keepLatest backups and that the
		// oldest one was deleted by the retention manager (the newest survive).
		t.Logf("[Level 1] Retention verification: tier2 should have exactly %d backup(s)", f.keepLatest)
		err = wait.For(
			checkTier2HasBackups(r, f.namespace, f.klioServer.Name, f.keepLatest),
			wait.WithTimeout(f.replicationTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "Level 1 failed: tier2 backup count verification failed")

		survivingNames, err := listTier2BackupNames(ctx, r, f.namespace, f.klioServer.Name)
		require.NoError(t, err, "Level 1 failed: could not list surviving tier2 backups")
		require.NotContains(t, survivingNames, oldestTier2BackupName,
			"Level 1 failed: the oldest backup should have been deleted by the retention manager")
		t.Logf("[Level 1] PASSED: tier2 has exactly %d backup(s) and the oldest (%s) was deleted",
			f.keepLatest, oldestTier2BackupName)

		// ==========================================
		// Level 1b: On-demand retention (klio retention apply)
		// ==========================================
		// Tighten the tier2 retention to keep a single backup and apply it
		// immediately with `klio retention apply`, without taking a new backup.
		// Only the newest backup must remain afterwards.
		newestBackupName := survivingNames[0]
		t.Log("[Level 1b] On-demand retention: tightening tier2 retention to 1 and running `klio retention apply`")
		updateTier2RetentionLatest(ctx, t, r, f.namespace, f.pluginConfigurationName, 1)

		instancePodName := f.backups[len(f.backups)-1].Status.InstanceID.PodName
		require.NotEmpty(t, instancePodName, "Level 1b failed: backup instance pod name should be set")

		// The `klio retention apply` command sends the policy from the pod's
		// mounted config, which the operator updates asynchronously after the
		// PluginConfiguration change. Re-running the (idempotent) command until
		// exactly one backup remains absorbs both the config propagation and the
		// asynchronous consumer processing.
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				if applyErr := runRetentionApply(ctx, r, f.namespace, instancePodName); applyErr != nil {
					t.Logf("[Level 1b] klio retention apply not ready yet: %v", applyErr)

					return false, nil
				}

				names, listErr := listTier2BackupNames(ctx, r, f.namespace, f.klioServer.Name)
				if listErr != nil {
					return false, nil //nolint:nilerr
				}

				return len(names) == 1, nil
			},
			wait.WithTimeout(f.replicationTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "Level 1b failed: on-demand retention did not converge to a single backup")

		finalNames, err := listTier2BackupNames(ctx, r, f.namespace, f.klioServer.Name)
		require.NoError(t, err, "Level 1b failed: could not list surviving tier2 backups")
		require.Equal(t, []string{newestBackupName}, finalNames,
			"Level 1b failed: only the newest backup should remain after `klio retention apply`")
		t.Logf("[Level 1b] PASSED: only the newest backup (%s) remains after apply", newestBackupName)

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

// checkTier2HasBackups checks if tier2 has exactly the expected number of backups.
// Returns (false, nil) on transient errors to allow the wait to continue retrying.
func checkTier2HasBackups(
	r *resources.Resources,
	namespace string,
	serverName string,
	expectedCount int,
) k8swait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		podName := serverName + klioPodSuffix

		// Use the Klio admin API to list backups
		var stdout, stderr bytes.Buffer
		klioCmd := []string{
			"klio", "admin", "list-backups",
		}

		if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, klioCmd, &stdout, &stderr); err != nil {
			// Return false without error to keep retrying on transient failures
			return false, nil //nolint:nilerr
		}

		// Parse JSON output as BackupList
		type BackupMetadata struct {
			Name        string            `json:"name"`
			ClusterName string            `json:"clusterName"`
			Annotations map[string]string `json:"annotations,omitempty"`
		}

		var backups []BackupMetadata
		if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
			// Return false without error to keep retrying on transient failures
			return false, nil //nolint:nilerr
		}

		// Count backups present in tier2 (those with the tier2 annotation)
		tier2Count := 0
		for i := range backups {
			if backups[i].Annotations[tier2AnnotationName] == presentAnnotationValue {
				tier2Count++
			}
		}

		return tier2Count == expectedCount, nil
	}
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

// listTier2BackupNames returns the names of the backups currently present in
// tier2 (those carrying the tier2 annotation), ordered newest first by their
// start time.
func listTier2BackupNames(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	serverName string,
) ([]string, error) {
	podName := serverName + klioPodSuffix

	var stdout, stderr bytes.Buffer
	klioCmd := []string{"klio", "admin", "list-backups"}
	if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, klioCmd, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("failed to list backups: %w; stderr: %s", err, stderr.String())
	}

	type backupMetadata struct {
		Name        string            `json:"name"`
		StartedAt   int64             `json:"startedAt"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}

	var backups []backupMetadata
	if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
		return nil, fmt.Errorf("failed to parse backup list: %w", err)
	}

	slices.SortFunc(backups, func(a, b backupMetadata) int {
		return cmp.Compare(b.StartedAt, a.StartedAt)
	})

	names := make([]string, 0, len(backups))
	for i := range backups {
		if backups[i].Annotations[tier2AnnotationName] == presentAnnotationValue {
			names = append(names, backups[i].Name)
		}
	}

	return names, nil
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
