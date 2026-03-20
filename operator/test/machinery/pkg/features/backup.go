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

	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
)

// BackupAssertFunc is a function that performs assertions after a backup completes.
type BackupAssertFunc func(ctx context.Context, t *testing.T, cfg *envconf.Config, backup *cnpgv1.Backup)

// BackupFeature defines a feature for testing backups in the CloudNativePG operator.
type BackupFeature struct {
	name                string
	setup               types.StepFunc
	teardown            types.StepFunc
	backup              *cnpgv1.Backup
	backupTimeout       time.Duration
	backupCheckInterval time.Duration
	postBackupAssert    BackupAssertFunc
}

// BackupFeatureConfig holds the configuration for creating a backup feature test.
type BackupFeatureConfig struct {
	// Name of the backup feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
	// Backup resource to be created and tested.
	Backup *cnpgv1.Backup
	// BackupTimeout is the timeout for backup (defaults to 1 minute).
	BackupTimeout time.Duration
	// BackupCheckInterval is the interval for checking backup status (defaults to 10 seconds).
	BackupCheckInterval time.Duration
	// PostBackupAssert is an optional function to run assertions after backup completes.
	PostBackupAssert BackupAssertFunc
}

// NewBackupFeature creates a new BackupFeature with the given configuration and default timeouts.
func NewBackupFeature(config BackupFeatureConfig) *BackupFeature {
	if config.BackupTimeout <= 0 {
		// Default backupTimeout for backup operations
		config.BackupTimeout = 1 * time.Minute
	}
	if config.BackupCheckInterval <= 0 {
		// Default interval for checking backup status
		config.BackupCheckInterval = 10 * time.Second
	}

	return &BackupFeature{
		name:                config.Name,
		setup:               config.Setup,
		teardown:            config.Teardown,
		backup:              config.Backup,
		backupTimeout:       config.BackupTimeout,
		backupCheckInterval: config.BackupCheckInterval,
		postBackupAssert:    config.PostBackupAssert,
	}
}

// Name returns the name of the backup feature.
func (f *BackupFeature) Name() string {
	return f.name
}

// Setup initializes the backup feature test, setting up the necessary resources.
func (f *BackupFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the backup feature test, creating a backup and waiting for it to complete.
func (f *BackupFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running backup feature test")
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")
		require.NoError(t, r.Create(ctx, f.backup), "failed to create backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, f.backup),
			wait.WithTimeout(f.backupTimeout),
			wait.WithInterval(f.backupCheckInterval),
		)
		require.NoError(t, err, "backup not completed")

		if f.postBackupAssert != nil {
			f.postBackupAssert(ctx, t, cfg, f.backup)
		}

		return ctx
	}
}

// Teardown cleans up resources after the test is run.
func (f *BackupFeature) Teardown() types.StepFunc {
	return f.teardown
}
