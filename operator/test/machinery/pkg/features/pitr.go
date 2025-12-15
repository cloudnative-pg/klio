package features

import (
	"context"
	"strconv"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/postgres"
)

// PitrFeature defines a feature for testing the PITR of a backup in the CloudNativePG operator.
type PitrFeature struct {
	*RecoveryFeature
}

// NewPitrFeature creates a new PitrFeature with the given configuration and default timeouts.
func NewPitrFeature(config RecoveryFeatureConfig) *PitrFeature {
	return &PitrFeature{
		RecoveryFeature: NewRecoveryFeature(config),
	}
}

// Run executes the PITR feature test, defining a recovery targetTime, recovering a Cluster
// from a backup defining and waiting for it to complete.
func (f *PitrFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running PITR feature test")
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

		// Insert some test data in the source Cluster
		_, err = postgres.ExecPostgresQuery(ctx, r, f.sourcePrimaryPod, "postgres",
			"CREATE TABLE numbers AS SELECT generate_series(1, 1000) AS x;")
		require.NoError(t, err, "failed to create table")

		// Get the targetTime
		targetTimestamp, err := postgres.ExecPostgresQuery(ctx, r, f.sourcePrimaryPod, "postgres",
			"SELECT now();")
		require.NoError(t, err, "failed to get the current timestamp")

		// Drop a table
		_, err = postgres.ExecPostgresQuery(ctx, r, f.sourcePrimaryPod, "postgres",
			"DROP TABLE numbers;")
		require.NoError(t, err, "failed to drop table")
		require.NoError(t, postgres.CheckpointAndSwitchWal(ctx, r, f.sourcePrimaryPod),
			"failed to checkpoint and switch WAL")

		// Mutate recovery cluster
		for _, mutate := range f.mutateRecoveryCluster {
			require.NoError(t, mutate(ctx, f.recoveryCluster, r), "failed to mutate recovery cluster")
		}

		// Recovery
		f.recoveryCluster.Spec.Bootstrap.Recovery.RecoveryTarget = &cnpgv1.RecoveryTarget{
			TargetTime: targetTimestamp,
		}
		require.NoError(t, r.Create(ctx, f.recoveryCluster), "failed to create a recovery cluster")
		err = wait.For(
			machineryConditions.ClusterIsReady(r, f.recoveryCluster),
			wait.WithTimeout(f.recoveryTimeout),
			wait.WithInterval(f.recoveryCheckInterval),
		)
		require.NoError(t, err, "recovery Cluster not ready")

		// Verify data in the recovered Cluster
		var recoveryPrimaryPod corev1.Pod
		require.NoError(t,
			r.Get(ctx, f.recoveryCluster.Status.CurrentPrimary, f.recoveryCluster.Namespace, &recoveryPrimaryPod),
			"failed to get the recovery Cluster primary pod")
		out, err := postgres.ExecPostgresQuery(ctx, r, &recoveryPrimaryPod, "postgres",
			"SELECT COUNT(*) FROM numbers;")
		require.NoError(t, err, "failed to verify data")
		countedEntries, err := strconv.Atoi(out)
		require.NoError(t, err)
		require.Equal(t, 1000, countedEntries)

		return ctx
	}
}
