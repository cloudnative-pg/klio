package features

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/postgres"
)

// ReplicaClusterFeature defines a feature for testing the replication between a source Cluster and a Replica Cluster
// in the CloudNativePG operator, recovering the WAL files from the archive.
type ReplicaClusterFeature struct {
	*RecoveryFeature
}

// NewReplicaClusterFeature creates a new ReplicaClusterFeature with the given configuration and default timeouts.
func NewReplicaClusterFeature(config RecoveryFeatureConfig) *ReplicaClusterFeature {
	return &ReplicaClusterFeature{
		RecoveryFeature: NewRecoveryFeature(config),
	}
}

// Run executes the Replica Cluster feature test:
// 1. Recovers a Cluster from a backup
// 2. Applies changes in the source Cluster
// 3. Waits for the changes to be correctly applied in the replica Cluster.
func (f *ReplicaClusterFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running replica cluster feature test")
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Take a backup
		require.NoError(t, r.Create(ctx, f.backup), "failed to create backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, f.backup),
			wait.WithTimeout(f.backupTimeout),
			wait.WithInterval(f.backupCheckInterval),
		)
		require.NoError(t, err, "backup not completed")

		// Recovery
		f.recoveryCluster.Spec.ReplicaCluster = &cnpgv1.ReplicaClusterConfiguration{
			Source:  f.recoveryCluster.Spec.Bootstrap.Recovery.Source,
			Enabled: ptr.To(true),
		}

		// Mutate recovery cluster
		for _, mutate := range f.mutateRecoveryCluster {
			require.NoError(t, mutate(ctx, f.recoveryCluster, r), "failed to mutate recovery cluster")
		}

		require.NoError(t, r.Create(ctx, f.recoveryCluster), "failed to create a recovery cluster")
		err = wait.For(
			machineryConditions.ClusterIsReady(r, f.recoveryCluster),
			wait.WithTimeout(f.recoveryTimeout),
			wait.WithInterval(f.recoveryCheckInterval),
		)
		require.NoError(t, err, "recovery Cluster not ready")

		// Insert some test data in the source Cluster
		_, err = postgres.ExecPostgresQuery(ctx, r, f.sourcePrimaryPod, "postgres",
			"CREATE TABLE numbers AS SELECT generate_series(1, 1000) AS x;")
		require.NoError(t, err, "failed to create table")
		require.NoError(t, postgres.CheckpointAndSwitchWal(ctx, r, f.sourcePrimaryPod),
			"failed to checkpoint and switch WAL")

		// Verify replica Cluster is replicating from source Cluster
		var recoveryPrimaryPod corev1.Pod
		require.NoError(t,
			r.Get(ctx, f.recoveryCluster.Status.CurrentPrimary, f.recoveryCluster.Namespace, &recoveryPrimaryPod),
			"failed to get the replica Cluster primary pod")

		primaryLSN, err := postgres.ExecPostgresQuery(ctx, r, f.sourcePrimaryPod, "postgres",
			"SELECT replay_lsn FROM pg_stat_replication WHERE application_name = 'klio';")
		require.NoError(t, err, "failed to gather replay_lsn from source Cluster's primary")

		query := fmt.Sprintf(
			"SELECT CASE WHEN pg_lsn_ge(pg_last_wal_replay_lsn(),'%s') THEN 'true'::TEXT ELSE 'false'::TEXT END;",
			primaryLSN)
		assert.Eventually(t, func() bool {
			replicating, err := postgres.ExecPostgresQuery(ctx, r, &recoveryPrimaryPod, "postgres", query)
			require.NoError(t, err, "failed to compare LSN in the replica Cluster's primary")

			return replicating == "true"
		}, 10*time.Second, 2*time.Second, "Replica Cluster is in sync with the source Cluster")

		// Verify data are replicated in the Replica Cluster
		out, err := postgres.ExecPostgresQuery(ctx, r, &recoveryPrimaryPod, "postgres",
			"SELECT COUNT(*) FROM numbers;")
		require.NoError(t, err, "failed to verify data")
		countedEntries, err := strconv.Atoi(out)
		require.NoError(t, err)
		require.Equal(t, 1000, countedEntries)

		return ctx
	}
}
