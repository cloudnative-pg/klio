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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
	klioConditions "github.com/cloudnative-pg/klio/operator/test/klio/conditions"
	"github.com/cloudnative-pg/klio/operator/test/klio/infra"
	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/namespaces"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/postgres"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

const (
	// globalCompressionAlgorithm is the repository-wide compression policy set
	// on the Server. Clusters without an override inherit it.
	globalCompressionAlgorithm = "s2-default"

	// clusterTier1CompressionAlgorithmRound1/Round2 and
	// clusterTier2CompressionAlgorithmRound1/Round2 are the per-cluster
	// compression policies set on the PluginConfiguration for tier1 and tier2.
	// The PluginConfiguration is changed from "round1" to "round2" between two
	// backups, so each tier ends up with two backups compressed under a
	// different algorithm. All four values differ from each other and
	// from globalCompressionAlgorithm, so we can prove both that the override
	// takes precedence over the global policy and that recovery does not
	// depend on which algorithm compressed a given backup.
	clusterTier1CompressionAlgorithmRound1 = "zstd"
	clusterTier1CompressionAlgorithmRound2 = "pgzip"
	clusterTier2CompressionAlgorithmRound1 = "gzip"
	clusterTier2CompressionAlgorithmRound2 = "zstd-fastest"

	// globalCompressionMinSize and clusterCompressionMinSize exercise the
	// optional minSize bound on the Server (global) and the PluginConfiguration
	// (per-cluster) respectively. They differ so the override is provable.
	globalCompressionMinSize  = 2048
	clusterCompressionMinSize = 4096

	// compressionExternalClusterName is the CNPG external cluster name used by
	// every recovery cluster to refer back to the source cluster's backups.
	compressionExternalClusterName = "source-cluster"

	// compressionRound1RowCount and compressionRound2RowCount are the expected
	// row counts of the "numbers" table at backup round1 and round2
	// respectively: round2 appends to round1's data, so each backup's
	// recovered row count proves which round it was taken in.
	compressionRound1RowCount = 1000
	compressionRound2RowCount = 2000

	// tier1KopiaConfigPattern and tier2RWKopiaConfigPattern glob the Kopia
	// config files the server creates at startup, via their companion password
	// files. tier1 is the local filesystem repository; tier2 is the read-write
	// S3 repository.
	tier1KopiaConfigPattern   = "/tmp/kopiaconfig_tier1_*.kopia-password"
	tier2RWKopiaConfigPattern = "/tmp/kopiaconfig_tier2_rw_*.kopia-password"

	compressionServerContainerName = "server"
	compressionKlioPodSuffix       = "-klio-0"
)

// compressionRecoveryTarget bundles the PluginConfiguration and Cluster used
// to recover one specific backup.
type compressionRecoveryTarget struct {
	pluginConfiguration *kliov1alpha1.PluginConfiguration
	cluster             *cnpgv1.Cluster
}

// compressionScenario contains all resources needed for compression testing.
// It reuses the tier2 infrastructure so that both the tier2 global policy
// (Server) and the tier2 per-cluster policy (PluginConfiguration) can be
// verified against the same Kopia repository.
type compressionScenario struct {
	namespace *corev1.Namespace

	// tier2Infra bundles the shared RustFS + Klio Server (tier2) bring-up.
	tier2Infra infra.Tier2

	// klioServer is a shortcut to tier2Infra.KlioServer used by the verifier.
	klioServer *kliov1alpha1.Server

	// Source cluster
	cnpgCluster             *cnpgv1.Cluster
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration
	backupRound1            *cnpgv1.Backup
	backupRound2            *cnpgv1.Backup

	// Tier1 recovery: a plugin configuration pointing at the same
	// tier1-capable klioServer, with tier2 disabled so recovery is forced
	// onto tier1. It recovers the latest backup (backupRound2).
	tier1Recovery compressionRecoveryTarget

	// Tier2 recovery: a second, tier2-only, read-only Server, created only
	// once both backups have reached tier2. It also recovers the latest
	// backup (backupRound2).
	tier2RecoveryServerCertificate   *certmanagerv1.Certificate
	tier2RecoveryServerCACertificate *certmanagerv1.Certificate
	tier2RecoveryServerCAIssuer      *certmanagerv1.Issuer
	tier2RecoveryUserCertificate     *certmanagerv1.Certificate
	tier2RecoveryServer              *kliov1alpha1.Server
	tier2Recovery                    compressionRecoveryTarget

	name             string
	sourcePrimaryPod corev1.Pod
}

// Setup creates all resources for compression testing.
func (s *compressionScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for compression feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	createNamespace(ctx, t, r, s.namespace)

	// Bring up the shared RustFS + tier2 Klio Server infrastructure.
	s.tier2Infra.ParallelSetup(ctx, t, r)

	t.Logf("Deploying source CNPG cluster and plugin configuration...")
	require.NoError(t, r.Create(ctx, s.klioPluginConfiguration),
		"failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG source cluster")

	require.NoError(t, wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	), "source cluster not ready")

	require.NoError(
		t,
		r.Get(ctx, s.cnpgCluster.Status.CurrentPrimary, s.namespace.Name, &s.sourcePrimaryPod),
		"failed to get the current primary pod",
	)

	t.Logf("All resources ready for compression feature: %s", s.name)

	return ctx
}

// Teardown deletes all resources.
func (s *compressionScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for compression feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	namespaces.DumpNamespaceOnFailure(ctx, t, r, testCfg.LogDir, s.namespace.Name, testconfig.DumpedKinds())
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for compression feature: %s", s.name)

	return ctx
}

// kopiaConfigFile discovers an ephemeral Kopia config file the server created
// at startup, by globbing its companion password file with the passed pattern.
func (s *compressionScenario) kopiaConfigFile(
	ctx context.Context,
	r *resources.Resources,
	pattern string,
) (string, error) {
	podName := s.klioServer.Name + compressionKlioPodSuffix

	var stdout, stderr bytes.Buffer
	findCmd := []string{"sh", "-c", "ls " + pattern + " 2>/dev/null"}
	if err := r.ExecInPod(
		ctx, s.namespace.Name, podName, compressionServerContainerName, findCmd, &stdout, &stderr,
	); err != nil {
		return "", fmt.Errorf("failed to locate kopia config %q: %w; stderr: %s", pattern, err, stderr.String())
	}

	passwordFile := strings.TrimSpace(stdout.String())
	if passwordFile == "" {
		return "", fmt.Errorf("no kopia config file found matching %q", pattern)
	}

	return strings.TrimSuffix(passwordFile, ".kopia-password"), nil
}

// compressionOfPolicy runs `kopia policy show <target> --json` against the
// passed config and returns the effective compression settings.
func (s *compressionScenario) compressionOfPolicy(
	ctx context.Context,
	r *resources.Resources,
	configFile string,
	target string,
) (effectiveCompression, error) {
	podName := s.klioServer.Name + compressionKlioPodSuffix

	var stdout, stderr bytes.Buffer
	showCmd := []string{
		"kopia", "policy", "show", target,
		"--disable-file-logging",
		"--config-file=" + configFile,
		"--json",
	}
	if err := r.ExecInPod(
		ctx, s.namespace.Name, podName, compressionServerContainerName, showCmd, &stdout, &stderr,
	); err != nil {
		return effectiveCompression{},
			fmt.Errorf("failed to show kopia policy for %q: %w; stderr: %s", target, err, stderr.String())
	}

	var policy struct {
		Compression effectiveCompression `json:"compression"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &policy); err != nil {
		return effectiveCompression{}, fmt.Errorf("failed to parse kopia policy %q: %w", stdout.String(), err)
	}

	return policy.Compression, nil
}

// effectiveCompression is the subset of a Kopia policy's compression settings
// that the test asserts on.
type effectiveCompression struct {
	CompressorName string `json:"compressorName"`
	MinSize        int64  `json:"minSize"`
}

// clusterPolicyTarget finds the "user@host" policy target for the cluster in
// the tier2 repository. The per-cluster policy only exists once a backup has
// been relayed to tier2, so this returns an empty string until then.
func (s *compressionScenario) clusterPolicyTarget(
	ctx context.Context,
	r *resources.Resources,
	configFile string,
) (string, error) {
	podName := s.klioServer.Name + compressionKlioPodSuffix

	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"kopia", "policy", "list",
		"--disable-file-logging",
		"--config-file=" + configFile,
		"--json",
	}
	if err := r.ExecInPod(
		ctx, s.namespace.Name, podName, compressionServerContainerName, listCmd, &stdout, &stderr,
	); err != nil {
		return "", fmt.Errorf("failed to list kopia policies: %w; stderr: %s", err, stderr.String())
	}

	var policies []struct {
		Target struct {
			Host string `json:"host"`
			User string `json:"userName"`
		} `json:"target"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &policies); err != nil {
		return "", fmt.Errorf("failed to parse kopia policy list %q: %w", stdout.String(), err)
	}

	for _, p := range policies {
		if p.Target.Host == s.cnpgCluster.Name && p.Target.User != "" {
			return p.Target.User + "@" + p.Target.Host, nil
		}
	}

	return "", nil
}

// verifyTierCompression asserts that, for the repository selected by
// configPattern, the global compression policy matches the Server setting and
// the cluster's own source policy matches the passed per-cluster override.
func (s *compressionScenario) verifyTierCompression(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	tier string,
	configPattern string,
	expectedClusterAlgorithm string,
) {
	t.Helper()

	configFile, err := s.kopiaConfigFile(ctx, r, configPattern)
	require.NoError(t, err, "[%s] failed to discover kopia config file", tier)

	// The global policy is applied when the server starts, so it is already
	// present. Verify its algorithm and minSize match what the Server requested.
	t.Logf("[%s] verifying the global compression policy...", tier)
	globalCompression, err := s.compressionOfPolicy(ctx, r, configFile, "--global")
	require.NoError(t, err, "[%s] failed to read the global compression policy", tier)
	require.Equal(t, globalCompressionAlgorithm, globalCompression.CompressorName,
		"[%s] unexpected global compression algorithm", tier)
	require.Equal(t, int64(globalCompressionMinSize), globalCompression.MinSize,
		"[%s] unexpected global compression minSize", tier)

	// The per-cluster policy is applied during the backup, so poll until it
	// appears with the expected algorithm and minSize.
	t.Logf("[%s] waiting for the per-cluster compression policy to become %q...", tier, expectedClusterAlgorithm)
	var clusterCompression effectiveCompression
	err = wait.For(
		func(ctx context.Context) (bool, error) {
			target, err := s.clusterPolicyTarget(ctx, r, configFile)
			if err != nil || target == "" {
				return false, err
			}
			clusterCompression, err = s.compressionOfPolicy(ctx, r, configFile, target)
			if err != nil {
				return false, err
			}

			return clusterCompression.CompressorName == expectedClusterAlgorithm &&
				clusterCompression.MinSize == clusterCompressionMinSize, nil
		},
		wait.WithTimeout(3*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err,
		"[%s] per-cluster compression policy did not become %q/minSize=%d (last seen %+v)",
		tier, expectedClusterAlgorithm, clusterCompressionMinSize, clusterCompression)

	t.Logf("[%s] compression policies verified: global=%+v, cluster=%+v",
		tier, globalCompression, clusterCompression)
}

// waitForBackupCount waits until `klio admin list-backups` on the source Klio
// server pod reports exactly expectedCount backups, proving every backup
// taken so far is registered before recovery begins.
func (s *compressionScenario) waitForBackupCount(
	t *testing.T,
	r *resources.Resources,
	expectedCount int,
) {
	t.Helper()

	podName := s.klioServer.Name + compressionKlioPodSuffix
	err := wait.For(
		func(ctx context.Context) (bool, error) {
			var stdout, stderr bytes.Buffer
			cmd := []string{"klio", "admin", "list-backups"}
			if err := r.ExecInPod(
				ctx, s.namespace.Name, podName, compressionServerContainerName, cmd, &stdout, &stderr,
			); err != nil {
				// Keep retrying on transient failures.
				return false, nil //nolint:nilerr
			}

			var backups []struct{}
			if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
				return false, nil //nolint:nilerr
			}

			return len(backups) == expectedCount, nil
		},
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "backup count did not reach %d before recovery", expectedCount)
}

// verifyRecoveredRowCount asserts that the "numbers" table created before the
// backup has the expected row count in the recovered Cluster.
func verifyRecoveredRowCount(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	cluster *cnpgv1.Cluster,
	expectedCount int,
) {
	t.Helper()

	var primaryPod corev1.Pod
	require.NoError(t, r.Get(ctx, cluster.Status.CurrentPrimary, cluster.Namespace, &primaryPod),
		"failed to get the recovered cluster primary pod")

	out, err := postgres.ExecPostgresQuery(ctx, r, &primaryPod, "postgres", "SELECT COUNT(*) FROM numbers;")
	require.NoError(t, err, "failed to verify recovered data")

	count, err := strconv.Atoi(out)
	require.NoError(t, err, "failed to parse row count")
	require.Equal(t, expectedCount, count, "cluster %s recovered with unexpected row count", cluster.Name)
}

// createAndVerifyRecovery creates the recovery Cluster for target, waits for
// it to become ready, and asserts its "numbers" table has expectedCount rows.
// The target's PluginConfiguration must already exist.
func createAndVerifyRecovery(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	label string,
	target compressionRecoveryTarget,
	expectedCount int,
) {
	t.Helper()

	t.Logf("Recovering %s (expecting %d rows)...", label, expectedCount)
	require.NoError(t, r.Create(ctx, target.cluster), "failed to create %s recovery cluster", label)
	err := wait.For(
		machineryConditions.ClusterIsReady(r, target.cluster),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "%s recovery cluster not ready", label)
	verifyRecoveredRowCount(ctx, t, r, target.cluster, expectedCount)
	t.Logf("%s recovery verified", label)
}

// CompressionFeature verifies the global and per-cluster compression policies.
type CompressionFeature struct {
	name     string
	scenario *compressionScenario
}

// Name returns the name of the feature.
func (f *CompressionFeature) Name() string {
	return f.name
}

// Setup initializes the test resources.
func (f *CompressionFeature) Setup() types.StepFunc {
	return f.scenario.Setup
}

// Run verifies that the repository-wide compression policy configured on the
// Server is applied globally, and that the per-cluster policy configured on
// the PluginConfiguration overrides it for the cluster's own source. Two
// backups are taken per tier, changing the per-cluster algorithm in between,
// so each tier ends up with backups compressed under different algorithms.
// The latest backup is then recovered from both tier1 and tier2, checked for
// data integrity, proving that recovery works on top of that mixed-algorithm
// history rather than depending on a single algorithm being used throughout.
func (f *CompressionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running compression policy test")

		// The per-cluster policies must differ from the global one and from
		// each other, otherwise the override assertions below would pass
		// vacuously and mixed compression algorithms would not be exercised.
		for _, algorithm := range []string{
			clusterTier1CompressionAlgorithmRound1,
			clusterTier1CompressionAlgorithmRound2,
			clusterTier2CompressionAlgorithmRound1,
			clusterTier2CompressionAlgorithmRound2,
		} {
			require.NotEqual(t, globalCompressionAlgorithm, algorithm,
				"test misconfigured: per-cluster algorithm %q must differ from the global one", algorithm)
		}
		require.NotEqual(t, clusterTier1CompressionAlgorithmRound1, clusterTier1CompressionAlgorithmRound2,
			"test misconfigured: tier1 round1 and round2 algorithms must differ")
		require.NotEqual(t, clusterTier2CompressionAlgorithmRound1, clusterTier2CompressionAlgorithmRound2,
			"test misconfigured: tier2 round1 and round2 algorithms must differ")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		s := f.scenario

		// Round 1: back up under clusterTier1/2CompressionAlgorithmRound1.
		t.Log("Inserting round1 test data in the source cluster...")
		_, err = postgres.ExecPostgresQuery(ctx, r, &s.sourcePrimaryPod, "postgres",
			fmt.Sprintf("CREATE TABLE numbers AS SELECT generate_series(1, %d) AS x;", compressionRound1RowCount))
		require.NoError(t, err, "failed to create table")
		require.NoError(t, postgres.CheckpointAndSwitchWal(ctx, r, &s.sourcePrimaryPod),
			"failed to checkpoint and switch WAL")

		t.Log("Creating backup 1 (round1 compression algorithms)...")
		require.NoError(t, r.Create(ctx, s.backupRound1), "failed to create backup 1")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, s.backupRound1),
			wait.WithTimeout(3*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "backup 1 not completed")

		s.verifyTierCompression(ctx, t, r, "tier1", tier1KopiaConfigPattern, clusterTier1CompressionAlgorithmRound1)
		s.verifyTierCompression(ctx, t, r, "tier2", tier2RWKopiaConfigPattern, clusterTier2CompressionAlgorithmRound1)

		// Switch the source cluster's compression algorithms and wait for the
		// change to actually take effect (the klio-plugin sidecar restarts
		// once the new PluginConfiguration Secret has propagated).
		t.Log("Updating PluginConfiguration to round2 compression algorithms...")
		var primaryPod corev1.Pod
		require.NoError(t, r.Get(ctx, s.cnpgCluster.Status.CurrentPrimary, s.namespace.Name, &primaryPod),
			"failed to get the current primary pod")
		var initialRestartCount int32
		containerFound := false
		for _, containerStatus := range primaryPod.Status.InitContainerStatuses {
			if containerStatus.Name == cnpgi.KlioPluginContainerName {
				initialRestartCount = containerStatus.RestartCount
				containerFound = true

				break
			}
		}
		require.True(t, containerFound, "klio-plugin init container not found in primary pod")

		var currentPC kliov1alpha1.PluginConfiguration
		require.NoError(t, r.Get(ctx, s.klioPluginConfiguration.Name, s.namespace.Name, &currentPC),
			"failed to get PluginConfiguration")
		currentPC.Spec.Tier1.Compression.Algorithm = clusterTier1CompressionAlgorithmRound2
		currentPC.Spec.Tier2.Compression.Algorithm = clusterTier2CompressionAlgorithmRound2
		require.NoError(t, r.Update(ctx, &currentPC), "failed to update PluginConfiguration")

		err = wait.For(
			klioConditions.PluginConfigurationHasCondition(
				r,
				s.klioPluginConfiguration,
				kliov1alpha1.PluginConfigurationConditionConfigurationApplied,
				metav1.ConditionTrue,
				currentPC.Generation,
			),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(5*time.Second),
		)
		require.NoError(t, err, "ConfigurationApplied condition not set correctly")

		err = wait.For(
			machineryConditions.InitContainerHasRestarted(
				r,
				s.cnpgCluster.Status.CurrentPrimary,
				s.namespace.Name,
				cnpgi.KlioPluginContainerName,
				initialRestartCount,
			),
			wait.WithTimeout(3*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "klio-plugin init container did not restart after PluginConfiguration update")

		// Round 2: back up under clusterTier1/2CompressionAlgorithmRound2.
		t.Log("Inserting round2 test data in the source cluster...")
		_, err = postgres.ExecPostgresQuery(ctx, r, &s.sourcePrimaryPod, "postgres",
			fmt.Sprintf("INSERT INTO numbers SELECT generate_series(%d, %d) AS x;",
				compressionRound1RowCount+1, compressionRound2RowCount))
		require.NoError(t, err, "failed to insert additional rows")
		require.NoError(t, postgres.CheckpointAndSwitchWal(ctx, r, &s.sourcePrimaryPod),
			"failed to checkpoint and switch WAL")

		t.Log("Creating backup 2 (round2 compression algorithms)...")
		require.NoError(t, r.Create(ctx, s.backupRound2), "failed to create backup 2")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, s.backupRound2),
			wait.WithTimeout(3*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "backup 2 not completed")

		s.verifyTierCompression(ctx, t, r, "tier1", tier1KopiaConfigPattern, clusterTier1CompressionAlgorithmRound2)
		s.verifyTierCompression(ctx, t, r, "tier2", tier2RWKopiaConfigPattern, clusterTier2CompressionAlgorithmRound2)

		// Both tiers now hold two backups compressed under different
		// algorithms. Confirm both are registered before recovering: recovery
		// must land on top of the full mixed-algorithm history, not just the
		// first backup.
		t.Log("Verifying both backups are registered before recovery...")
		s.waitForBackupCount(t, r, 2)

		// Recover the latest backup (backupRound2) from each tier: with no
		// explicit RecoveryTarget, recovery defaults to the latest backup.
		t.Log("Recovering the latest backup from tier1...")
		require.NoError(t, r.Create(ctx, s.tier1Recovery.pluginConfiguration),
			"failed to create tier1 recovery plugin configuration")
		createAndVerifyRecovery(ctx, t, r, "tier1", s.tier1Recovery, compressionRound2RowCount)

		t.Log("Recovering the latest backup from tier2...")
		require.NoError(t, deployTier2RecoveryServer(ctx, r, s.namespace.Name, s.klioServer.Name, 2,
			&tier2RecoveryServerResources{
				RecoveryServerCertificate:   s.tier2RecoveryServerCertificate,
				RecoveryServerCACertificate: s.tier2RecoveryServerCACertificate,
				RecoveryServerCAIssuer:      s.tier2RecoveryServerCAIssuer,
				RecoveryUserCertificate:     s.tier2RecoveryUserCertificate,
				RecoveryServer:              s.tier2RecoveryServer,
				PluginConfigurationRecovery: s.tier2Recovery.pluginConfiguration,
			}), "failed to deploy tier2 recovery server")
		createAndVerifyRecovery(ctx, t, r, "tier2", s.tier2Recovery, compressionRound2RowCount)

		return ctx
	}
}

// Teardown cleans up resources after the test.
func (f *CompressionFeature) Teardown() types.StepFunc {
	return f.scenario.Teardown
}

// newCompressionRecoveryCluster builds a recovery Cluster derived from
// template (the source cluster), pointing at the PluginConfiguration named
// pluginConfigName.
func newCompressionRecoveryCluster(template *cnpgv1.Cluster, clusterName, pluginConfigName string) *cnpgv1.Cluster {
	cluster := template.DeepCopy()
	cluster.Name = clusterName
	cluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: compressionExternalClusterName,
		PluginConfiguration: &cnpgv1.PluginConfiguration{
			Name:    "klio.cnpg.io",
			Enabled: new(true),
			Parameters: map[string]string{
				klioconfig.PluginConfigurationRefParam: pluginConfigName,
			},
		},
	}}
	cluster.Spec.Bootstrap = &cnpgv1.BootstrapConfiguration{
		Recovery: &cnpgv1.BootstrapRecovery{
			Source: compressionExternalClusterName,
		},
	}
	cluster.Spec.Plugins = []cnpgv1.PluginConfiguration{}

	return cluster
}

// newCompressionTier1RecoveryTarget builds a plugin configuration pointing at
// the same tier1-capable Server as sourcePluginConfiguration, with tier2
// disabled so recovery is forced onto tier1, and a recovery Cluster
// referencing it.
func newCompressionTier1RecoveryTarget(
	sourcePluginConfiguration *kliov1alpha1.PluginConfiguration,
	cnpgCluster *cnpgv1.Cluster,
	pluginConfigName, clusterName string,
) compressionRecoveryTarget {
	pluginConfiguration := sourcePluginConfiguration.DeepCopy()
	pluginConfiguration.Name = pluginConfigName
	pluginConfiguration.Spec.Tier2 = nil

	return compressionRecoveryTarget{
		pluginConfiguration: pluginConfiguration,
		cluster:             newCompressionRecoveryCluster(cnpgCluster, clusterName, pluginConfiguration.Name),
	}
}

// newCompressionTier2RecoveryTarget builds a plugin configuration pointing at
// the shared tier2-only recovery Server, and a recovery Cluster referencing
// it.
func newCompressionTier2RecoveryTarget(
	namespace, sourceClusterName string,
	serverCertificate, clientCertificate *certmanagerv1.Certificate,
	cnpgCluster *cnpgv1.Cluster,
	pluginConfigName, clusterName string,
) compressionRecoveryTarget {
	pluginConfiguration := klio.GetPluginConfigurationObject(
		pluginConfigName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   clientCertificate,
			ClusterName:         sourceClusterName,
			EnableTier2Backup:   false,
			EnableTier2Recovery: true,
			Mode:                kliov1alpha1.ModeReadOnly,
		},
	)

	return compressionRecoveryTarget{
		pluginConfiguration: pluginConfiguration,
		cluster:             newCompressionRecoveryCluster(cnpgCluster, clusterName, pluginConfiguration.Name),
	}
}

// compressionTier2RecoveryServerResources bundles the certificates and Server
// for the shared, tier2-only, read-only recovery Server.
type compressionTier2RecoveryServerResources struct {
	serverCertificate   *certmanagerv1.Certificate
	serverCACertificate *certmanagerv1.Certificate
	serverCAIssuer      *certmanagerv1.Issuer
	userCertificate     *certmanagerv1.Certificate
	server              *kliov1alpha1.Server
}

// newCompressionTier2RecoveryServerResources builds the certificates and
// Server object for the shared, tier2-only, read-only recovery Server,
// created once both backups have reached tier2. clientCertCN is the identity
// ("user@host") the recovering client authenticates as; it must match the
// source cluster's own client certificate so it resolves to the same Kopia
// user whose backups are being recovered.
func newCompressionTier2RecoveryServerResources(
	namespace, serverName, clientCertName, clientCertCN string,
	issuer *certmanagerv1.Issuer,
	encOpts klio.EncryptionOptions,
	s3Opts klio.Tier2S3Options,
) compressionTier2RecoveryServerResources {
	caCertificate := certificates.GetCACertificateObject(serverName+"-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject(serverName+"-ca-issuer", namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject(serverName+"-server", namespace, []string{serverName}, caIssuer)
	userCertificate := certificates.GetUserCertificateObject(clientCertName, namespace, clientCertCN, caIssuer)

	server := klio.GetReadOnlyTier2ServerObject(
		serverName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				Image:              testCfg.ServerImage,
				StorageClass:       testCfg.StorageClass,
				TLSSecretName:      serverCertificate.Spec.SecretName,
				ClientCASecretName: caCertificate.Spec.SecretName,
				Encryption:         encOpts,
			},
			Tier2Encryption: encOpts,
			S3:              s3Opts,
		},
	)

	return compressionTier2RecoveryServerResources{
		serverCertificate:   serverCertificate,
		serverCACertificate: caCertificate,
		serverCAIssuer:      caIssuer,
		userCertificate:     userCertificate,
		server:              server,
	}
}

// newCompressionScenario creates a new compression test scenario.
func newCompressionScenario(name string, namespace string) *compressionScenario {
	const (
		cnpgClusterName = "pg-compression"

		klioServerName         = "klio"
		klioReadOnlyServerName = "klio-tier2-only"

		selfSignedIssuerName  = "selfsigned-issuer"
		caCertificateName     = klioServerName + "-ca"
		caIssuerName          = caCertificateName + "-issuer"
		serverCertificateName = klioServerName + "-server"
		cnpgClientCertName    = cnpgClusterName + "-client"

		recoveryClientCertName = cnpgClusterName + "-tier2-client"

		rustfsName                = "rustfs"
		rustfsSecretName          = rustfsName + "-secret"
		rustfsConfigMapName       = rustfsName + "-config"
		rustfsCreateBucketJobName = rustfsName

		encryptionSecretName = "encryption"
		encryptionPassword   = "testencryptionpassword123"

		pluginConfigurationName   = "klio-plugin-configuration"
		pluginConfigTier1Recovery = pluginConfigurationName + "-tier1-recovery"
		pluginConfigTier2Recovery = pluginConfigurationName + "-tier2-recovery"

		backupRound1Name = "test-backup-1"
		backupRound2Name = "test-backup-2"

		tier1RestoredClusterName = cnpgClusterName + "-tier1"
		tier2RestoredClusterName = cnpgClusterName + "-tier2"

		s3Prefix = "tier2"
	)

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject(selfSignedIssuerName, namespace)

	rustfsSecret := rustfs.GetRustFSSecret(rustfsSecretName, namespace)
	rustfsConfigMap := rustfs.GetRustFSConfigMap(rustfsConfigMapName, namespace)
	rustfsCertificate := rustfs.GetRustFSCertificate(rustfsName, namespace, issuer)
	rustfsService := rustfs.GetRustFSService(rustfsName, namespace)
	rustfsDeployment := rustfs.GetRustFSDeployment(rustfsName, namespace)
	rustfsCreateBucketJob := rustfs.GetRustFSCreateBucketJob(
		rustfsCreateBucketJobName, namespace, rustfs.RustFSBucketName)

	caCertificate := certificates.GetCACertificateObject(caCertificateName, namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject(caIssuerName, namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject(serverCertificateName, namespace, []string{klioServerName},
		issuer)
	userCertificate := certificates.GetUserCertificateObject(
		cnpgClientCertName, namespace, cnpgClientCertName+"@"+cnpgClusterName, caIssuer)

	ageSecrets := secrets.GetKlioAgeEncryptionSecrets(encryptionSecretName, namespace, encryptionPassword)
	encOpts := klio.EncryptionOptions{
		EncryptionKeySecretName: ageSecrets.EncryptionKeySecret.Name,
		EncryptionKeyFileName:   "encryption-key.age",
		IdentitySecretName:      ageSecrets.IdentitySecret.Name,
		IdentityFileName:        "identity.txt",
	}
	s3Opts := klio.Tier2S3Options{
		S3BucketName:          rustfs.RustFSBucketName,
		S3Prefix:              s3Prefix,
		S3Endpoint:            rustfs.GetRustFSEndpoint(rustfsName, namespace),
		S3Region:              rustfs.RustFSRegion,
		S3AccessKeySecretName: rustfsSecret.Name,
		S3SecretKeySecretName: rustfsSecret.Name,
		S3CABundleSecretName:  rustfsCertificate.Spec.SecretName,
	}

	klioServer := klio.GetServerWithTier2Object(
		klioServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				Image:              testCfg.ServerImage,
				StorageClass:       testCfg.StorageClass,
				TLSSecretName:      serverCertificate.Spec.SecretName,
				ClientCASecretName: caCertificate.Spec.SecretName,
				Encryption:         encOpts,
			},
			Tier2Encryption: encOpts,
			S3:              s3Opts,
		},
	)
	// Repository-wide (global) compression policy on the Server: every cluster
	// that does not override it inherits this policy.
	klioServer.Spec.Tier1.Compression = &kliov1alpha1.CompressionPolicy{
		Algorithm: globalCompressionAlgorithm,
		MinSize:   globalCompressionMinSize,
	}
	klioServer.Spec.Tier2.Compression = &kliov1alpha1.CompressionPolicy{
		Algorithm: globalCompressionAlgorithm,
		MinSize:   globalCompressionMinSize,
	}

	cnpgCluster := cnpg.GetCnpgClusterObject(
		cnpgClusterName, namespace, 1, pluginConfigurationName,
		cnpg.ClusterTemplateOptions{StorageClass: testCfg.StorageClass})

	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		pluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   userCertificate,
			ClusterName:         cnpgClusterName,
			EnableTier2Backup:   true,
			EnableTier2Recovery: false,
			Mode:                kliov1alpha1.ModeStandard,
		},
	)
	// Per-cluster compression policy overriding the Server global policy for
	// this cluster's own source. It starts at the round1 algorithms; Run()
	// switches it to the round2 algorithms between the two backups.
	klioPluginConfiguration.Spec.Tier1 = &kliov1alpha1.Tier1PluginConfiguration{
		Compression: &kliov1alpha1.CompressionPolicy{
			Algorithm: clusterTier1CompressionAlgorithmRound1,
			MinSize:   clusterCompressionMinSize,
		},
	}
	klioPluginConfiguration.Spec.Tier2.Compression = &kliov1alpha1.CompressionPolicy{
		Algorithm: clusterTier2CompressionAlgorithmRound1,
		MinSize:   clusterCompressionMinSize,
	}

	backupRound1 := cnpg.GetCnpgBackupObject(backupRound1Name, namespace, cnpgv1.BackupTargetPrimary, cnpgCluster)
	backupRound2 := cnpg.GetCnpgBackupObject(backupRound2Name, namespace, cnpgv1.BackupTargetPrimary, cnpgCluster)

	tier1Recovery := newCompressionTier1RecoveryTarget(
		klioPluginConfiguration, cnpgCluster, pluginConfigTier1Recovery, tier1RestoredClusterName)

	// Tier2 recovery: a second, tier2-only, read-only Server, created later
	// once both backups have reached tier2.
	tier2Server := newCompressionTier2RecoveryServerResources(
		namespace, klioReadOnlyServerName, recoveryClientCertName, cnpgClientCertName+"@"+cnpgClusterName,
		issuer, encOpts, s3Opts)

	tier2Recovery := newCompressionTier2RecoveryTarget(namespace, cnpgClusterName,
		tier2Server.serverCertificate, tier2Server.userCertificate, cnpgCluster,
		pluginConfigTier2Recovery, tier2RestoredClusterName)

	return &compressionScenario{
		namespace: namespaceObj,
		tier2Infra: infra.Tier2{
			Issuer:                issuer,
			RustfsSecret:          rustfsSecret,
			RustfsConfigMap:       rustfsConfigMap,
			RustfsCertificate:     rustfsCertificate,
			RustfsService:         rustfsService,
			RustfsDeployment:      rustfsDeployment,
			RustfsCreateBucketJob: rustfsCreateBucketJob,
			ServerCertificate:     serverCertificate,
			CaCertificate:         caCertificate,
			CaIssuer:              caIssuer,
			UserCertificate:       userCertificate,
			EncryptionSecret:      ageSecrets.EncryptionKeySecret,
			IdentitySecret:        ageSecrets.IdentitySecret,
			KlioServer:            klioServer,
		},
		klioServer:                       klioServer,
		cnpgCluster:                      cnpgCluster,
		klioPluginConfiguration:          klioPluginConfiguration,
		backupRound1:                     backupRound1,
		backupRound2:                     backupRound2,
		tier1Recovery:                    tier1Recovery,
		tier2RecoveryServerCertificate:   tier2Server.serverCertificate,
		tier2RecoveryServerCACertificate: tier2Server.serverCACertificate,
		tier2RecoveryServerCAIssuer:      tier2Server.serverCAIssuer,
		tier2RecoveryUserCertificate:     tier2Server.userCertificate,
		tier2RecoveryServer:              tier2Server.server,
		tier2Recovery:                    tier2Recovery,
		name:                             name,
	}
}

// Compression returns a Feature that verifies the global and per-cluster
// Kopia compression policies are applied, that changing the per-cluster
// algorithm between backups produces two backups per tier under different
// algorithms, and that the latest backup still recovers correctly,
// with data integrity, from both tier1 and tier2.
func Compression(namespace string) *CompressionFeature {
	return &CompressionFeature{
		name:     "Compression",
		scenario: newCompressionScenario("Compression", namespace),
	}
}
