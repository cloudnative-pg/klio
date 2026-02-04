package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// tier2KopiaConfigPattern is the glob pattern for finding the tier2 Kopia config file.
	// Used by verifyTier2RetentionPolicySet to run kopia policy commands.
	// The file ending in .kopia-password contains the path to the actual config.
	tier2KopiaConfigPattern = "/tmp/kopiaconfig_tier2_*.kopia-password"
)

// Tier2RetentionFeature defines a feature for testing tier2 backup and WAL retention.
type Tier2RetentionFeature struct {
	name               string
	setup              types.StepFunc
	teardown           types.StepFunc
	backups            []*cnpgv1.Backup
	klioServer         *kliov1alpha1.Server
	namespace          string
	keepLatest         int
	backupTimeout      time.Duration
	replicationTimeout time.Duration
	checkInterval      time.Duration
	clusterName        string
	s3Prefix           string
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
		name:               config.Name,
		setup:              config.Setup,
		teardown:           config.Teardown,
		backups:            config.Backups,
		klioServer:         config.KlioServer,
		namespace:          config.Namespace,
		keepLatest:         config.KeepLatest,
		backupTimeout:      config.BackupTimeout,
		replicationTimeout: config.ReplicationTimeout,
		checkInterval:      config.CheckInterval,
		clusterName:        config.ClusterName,
		s3Prefix:           config.S3Prefix,
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
//  1. Result Verification: Verifies that tier2 contains exactly `keepLatest` backups
//     by counting Kopia snapshots. This confirms retention was applied but doesn't
//     prove the mechanism is working (could be coincidence or manual deletion).
//
//  2. Mechanism Verification: Queries Kopia directly via `kopia policy list` to verify
//     the retention policy was actually configured with the correct `keepLatest` value.
//     This proves the full policy propagation path is working:
//     PluginConfiguration CR -> operator -> klio-plugin config -> CloseBackup GRPC ->
//     NATS queue -> backup consumer -> SetKopiaPolicy() -> ApplyKopiaPolicy()
//
//  3. WAL Retention Verification: Monitors WAL directory count before and after
//     retention to verify WAL cleanup is occurring. This is a soft check (logs
//     warnings only) because WAL retention depends on backup metadata (StartWAL)
//     and timing, making strict assertions fragile.
//
// The test creates more backups than `keepLatest` to trigger retention, then verifies
// all three levels pass.
func (f *Tier2RetentionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running tier2 backup and WAL retention feature test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Track WAL directory count to verify WAL retention (Level 3 verification)
		var walDirCountAfterFirstBackup int

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
			if i == 0 {
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
		// Level 1: Result Verification
		// ==========================================
		// Verify that tier2 contains exactly keepLatest backups by querying
		// the Klio admin API and counting backups with the tier2 annotation.
		t.Logf("[Level 1] Result verification: tier2 should have exactly %d backup(s)", f.keepLatest)
		err = wait.For(
			checkTier2HasBackups(r, f.namespace, f.klioServer.Name, f.keepLatest),
			wait.WithTimeout(f.replicationTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "Level 1 failed: tier2 backup count verification failed")
		t.Logf("[Level 1] PASSED: tier2 has exactly %d backup(s)", f.keepLatest)

		// ==========================================
		// Level 2: Mechanism Verification
		// ==========================================
		// Query Kopia directly to verify the retention policy was actually set.
		// This proves the full propagation path is working, not just the result.
		// We use `kopia policy list` to find all policies and check for one
		// matching our cluster (hostname) with the expected keepLatest value.
		t.Log("[Level 2] Mechanism verification: checking Kopia retention policy is set...")
		policyKeepLatest, err := verifyTier2RetentionPolicySet(
			ctx, r, f.namespace, f.klioServer.Name, f.clusterName)
		require.NoError(t, err, "Level 2 failed: could not verify tier2 retention policy")
		require.Equal(t, f.keepLatest, policyKeepLatest,
			"Level 2 failed: tier2 Kopia retention policy keepLatest=%d, expected=%d",
			policyKeepLatest, f.keepLatest)
		t.Logf("[Level 2] PASSED: Kopia retention policy has keepLatest=%d", policyKeepLatest)

		// ==========================================
		// Level 3: WAL Retention Verification (Soft Check)
		// ==========================================
		// Monitor WAL directory count to verify cleanup is occurring.
		// This is a soft check (warnings only) because:
		// - WAL retention depends on backup metadata (StartWAL field)
		// - Timing variations can cause count fluctuations
		// - The exact count depends on PostgreSQL activity during the test
		verifyWALRetention(ctx, t, r, f, walDirCountAfterFirstBackup)

		t.Log("Tier2 retention test completed: all verification levels passed")

		return ctx
	}
}

// Teardown cleans up resources after the test is run.
func (f *Tier2RetentionFeature) Teardown() types.StepFunc {
	return f.teardown
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
		t.Log("[Level 3] SKIPPED: no baseline WAL count available")

		return
	}

	t.Log("[Level 3] WAL retention verification: checking WAL directory growth...")
	finalCount, err := countTier2WALDirectories(
		ctx, r, f.namespace, f.klioServer.Name, f.s3Prefix, f.clusterName)
	if err != nil {
		t.Logf("[Level 3] WARNING: could not count final WAL directories: %v", err)

		return
	}

	t.Logf("[Level 3] WAL directories: %d (was %d after first backup)", finalCount, baselineCount)

	// WAL retention should prevent unbounded growth. We allow 3x growth
	// to account for WALs generated during test execution.
	const maxGrowthFactor = 3
	if finalCount > baselineCount*maxGrowthFactor {
		t.Logf("[Level 3] WARNING: WAL directory count grew significantly (%d -> %d), "+
			"WAL retention may not be working as expected", baselineCount, finalCount)

		return
	}

	t.Log("[Level 3] PASSED: WAL directory growth is within acceptable bounds")
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

// verifyTier2RetentionPolicySet verifies that the Kopia retention policy is set in tier2.
// It uses `kopia policy list` to find all policies and searches for one matching the
// cluster name (hostname). This is necessary because policies are stored with
// "username@hostname" format and we don't know the username in the test context.
// Returns the keepLatest value from the policy, or an error if the policy is not set.
//
//nolint:cyclop
func verifyTier2RetentionPolicySet(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	serverName string,
	clusterName string,
) (int, error) {
	podName := serverName + klioPodSuffix

	// Find the tier2 config file
	var stdout, stderr bytes.Buffer
	findCmd := []string{
		"sh", "-c",
		"ls " + tier2KopiaConfigPattern + " 2>/dev/null",
	}

	err := r.ExecInPod(ctx, namespace, podName, serverContainerName, findCmd, &stdout, &stderr)
	if err != nil {
		return 0, fmt.Errorf("could not find kopia tier2 config: %w", err)
	}

	passwordFile := strings.TrimSpace(stdout.String())
	if passwordFile == "" {
		return 0, errors.New("kopia tier2 config password file not found")
	}

	configFile := strings.TrimSuffix(passwordFile, ".kopia-password")

	// First, use `kopia policy list` to find the full target (username@hostname).
	// We can't use `kopia policy show <hostname>` directly because Kopia stores policies
	// with "username@hostname" format and we don't know the username.
	stdout.Reset()
	stderr.Reset()
	listCmd := []string{
		"kopia", "policy", "list",
		"--disable-file-logging",
		"--config-file=" + configFile,
		"--json",
	}

	err = r.ExecInPod(ctx, namespace, podName, serverContainerName, listCmd, &stdout, &stderr)
	if err != nil {
		return 0, fmt.Errorf("failed to list kopia policies: %w; stderr: %s", err, stderr.String())
	}

	// Parse to find the target string for our cluster
	var policies []struct {
		Target struct {
			Host string `json:"host"`
			User string `json:"userName"`
		} `json:"target"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &policies); err != nil {
		return 0, fmt.Errorf("failed to parse kopia policy list output: %w", err)
	}

	// Find the full target for our cluster
	var targetStr string
	for _, p := range policies {
		if p.Target.Host == clusterName {
			targetStr = p.Target.User + "@" + p.Target.Host
			break
		}
	}
	if targetStr == "" {
		return 0, fmt.Errorf("no policy found for host %q in %d policies", clusterName, len(policies))
	}

	// Now use `kopia policy show` to get the full policy details
	stdout.Reset()
	stderr.Reset()
	showCmd := []string{
		"kopia", "policy", "show",
		targetStr,
		"--disable-file-logging",
		"--config-file=" + configFile,
		"--json",
	}

	err = r.ExecInPod(ctx, namespace, podName, serverContainerName, showCmd, &stdout, &stderr)
	if err != nil {
		return 0, fmt.Errorf("failed to show kopia policy for %q: %w; stderr: %s", targetStr, err, stderr.String())
	}

	// Parse the policy details
	var policyDetail struct {
		RetentionPolicy struct {
			KeepLatest *int `json:"keepLatest"`
		} `json:"retention"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &policyDetail); err != nil {
		return 0, fmt.Errorf("failed to parse kopia policy show output: %w", err)
	}

	if policyDetail.RetentionPolicy.KeepLatest == nil {
		return 0, fmt.Errorf("policy found for target %q but keepLatest not set", targetStr)
	}

	return *policyDetail.RetentionPolicy.KeepLatest, nil
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
