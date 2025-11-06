package features

import (
	"strconv"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/postgres"
)

// RecoveryFeature defines a feature for testing recovery of a backup in the CloudNativePG operator.
type RecoveryFeature struct {
	name                  string
	setup                 types.StepFunc
	teardown              types.StepFunc
	sourcePrimaryPod      *corev1.Pod
	backup                *cnpgv1.Backup
	recoveryCluster       *cnpgv1.Cluster
	mutateRecoveryCluster []RecoveryClusterMutateFunc
	backupTimeout         time.Duration
	backupCheckInterval   time.Duration
	recoveryTimeout       time.Duration
	recoveryCheckInterval time.Duration
}

// RecoveryClusterMutateFunc is a function that mutates a recovery Cluster object.
type RecoveryClusterMutateFunc func(ctx context.Context, recoveryCluster *cnpgv1.Cluster, r *resources.Resources) error

// RecoveryFeatureConfig holds the configuration for creating a recovery feature test.
type RecoveryFeatureConfig struct {
	// Name of the recovery feature test
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
	// SourcePrimaryPod is the current primary of the Cluster to be backed up and recovered.
	// The cluster must be running before the feature Run() starts.
	SourcePrimaryPod *corev1.Pod
	// Backup resource to be created during the test.
	Backup *cnpgv1.Backup
	// RecoveryCluster to be created from the backup.
	RecoveryCluster *cnpgv1.Cluster
	// MutateRecoveryCluster can be used to inject exposed runtime fields into the recovery cluster.
	MutateRecoveryCluster []RecoveryClusterMutateFunc
	// BackupTimeout is the timeout for backup operations (defaults to 1 minute).
	BackupTimeout time.Duration
	// BackupCheckInterval is the interval for checking backup status for completion (defaults to 10 seconds).
	BackupCheckInterval time.Duration
	// RecoveryTimeout is the timeout for recovery operations (defaults to 2 minutes)
	RecoveryTimeout time.Duration
	// RecoveryCheckInterval is the interval for checking recovery status for completion (defaults to 10 seconds).
	RecoveryCheckInterval time.Duration
}

// NewRecoveryFeature creates a new RecoveryFeature with the given configuration and default timeouts.
func NewRecoveryFeature(config RecoveryFeatureConfig) *RecoveryFeature {
	if config.BackupTimeout <= 0 {
		// Default timeout for backup operations
		config.BackupTimeout = 1 * time.Minute
	}
	if config.BackupCheckInterval <= 0 {
		// Default interval for checking backup status
		config.BackupCheckInterval = 10 * time.Second
	}
	if config.RecoveryTimeout <= 0 {
		// Default timeout for recovery operations
		config.RecoveryTimeout = 2 * time.Minute
	}
	if config.RecoveryCheckInterval <= 0 {
		// Default interval for checking recovery status
		config.RecoveryCheckInterval = 10 * time.Second
	}

	return &RecoveryFeature{
		name:                  config.Name,
		setup:                 config.Setup,
		teardown:              config.Teardown,
		sourcePrimaryPod:      config.SourcePrimaryPod,
		backup:                config.Backup,
		recoveryCluster:       config.RecoveryCluster,
		mutateRecoveryCluster: config.MutateRecoveryCluster,
		backupTimeout:         config.BackupTimeout,
		backupCheckInterval:   config.BackupCheckInterval,
		recoveryTimeout:       config.RecoveryTimeout,
		recoveryCheckInterval: config.RecoveryCheckInterval,
	}
}

// Name returns the name of the recovery feature.
func (f *RecoveryFeature) Name() string {
	return f.name
}

// Setup initializes the recovery feature test, setting up the necessary resources.
func (f *RecoveryFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the recovery feature test, creating a Cluster from a backup and waiting for it to complete.
func (f *RecoveryFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running recovery feature test")
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Insert some test data in the source Cluster
		_, err = postgres.ExecPostgresQuery(ctx, r, f.sourcePrimaryPod, "postgres",
			"CREATE TABLE numbers AS SELECT generate_series(1, 1000) AS x;")
		require.NoError(t, err, "failed to create table")
		require.NoError(t, postgres.CheckpointAndSwitchWal(ctx, r, f.sourcePrimaryPod),
			"failed to checkpoint and switch WAL")

		// Take a backup
		require.NoError(t, r.Create(ctx, f.backup), "failed to create backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, f.backup),
			wait.WithTimeout(f.backupTimeout),
			wait.WithInterval(f.backupCheckInterval),
		)
		require.NoError(t, err, "backup not completed")

		// Mutate recovery cluster
		for _, mutate := range f.mutateRecoveryCluster {
			require.NoError(t, mutate(ctx, f.recoveryCluster, r), "failed to mutate recovery cluster")
		}

		// Recovery
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

// Teardown cleans up resources after the test is run.
func (f *RecoveryFeature) Teardown() types.StepFunc {
	return f.teardown
}
