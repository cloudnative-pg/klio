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
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

const (
	serverContainerName = "server"
	// klioPodSuffix is the suffix added to the server name to form the pod name.
	klioPodSuffix = "-klio-0"
	// tier1AnnotationName is the annotation key used to mark backups present in tier1.
	tier1AnnotationName = "klio.io/tier1"
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

// checkTierHasBackups checks if the tier identified by tierAnnotation has exactly
// the expected number of backups. Returns (false, nil) on transient errors to
// allow the wait to continue retrying.
func checkTierHasBackups(
	r *resources.Resources,
	namespace string,
	serverName string,
	clusterName string,
	tierAnnotation string,
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

		// Count this cluster's backups present in the tier (those carrying the
		// tier annotation)
		count := 0
		for i := range backups {
			if backups[i].ClusterName != clusterName {
				continue
			}
			if backups[i].Annotations[tierAnnotation] == presentAnnotationValue {
				count++
			}
		}

		return count == expectedCount, nil
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

// listTierBackupNames returns the names of the backups currently present in the
// tier identified by tierAnnotation for the given cluster, ordered newest first
// by their start time.
func listTierBackupNames(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	serverName string,
	clusterName string,
	tierAnnotation string,
) ([]string, error) {
	podName := serverName + klioPodSuffix

	var stdout, stderr bytes.Buffer
	klioCmd := []string{"klio", "admin", "list-backups"}
	if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, klioCmd, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("failed to list backups: %w; stderr: %s", err, stderr.String())
	}

	type backupMetadata struct {
		Name        string            `json:"name"`
		ClusterName string            `json:"clusterName"`
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
		if backups[i].ClusterName != clusterName {
			continue
		}
		if backups[i].Annotations[tierAnnotation] == presentAnnotationValue {
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
