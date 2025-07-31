package features

import (
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"
)

// BackupFeature defines a feature for testing backups in the CloudNativePG operator.
type BackupFeature struct {
	name     string
	setup    func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context
	backup   *cnpgv1.Backup
	teardown func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context
	timeout  time.Duration
	interval time.Duration
}

// NewBackupFeature creates a new instance of BackupFeature.
func NewBackupFeature(
	name string,
	setup func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context,
	backup *cnpgv1.Backup,
	teardown func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context,
	options ...func(*BackupFeature),
) *BackupFeature {
	bf := &BackupFeature{
		name:     name,
		setup:    setup,
		backup:   backup,
		teardown: teardown,
		timeout:  2 * time.Minute,  // default
		interval: 10 * time.Second, // default
	}
	for _, opt := range options {
		opt(bf)
	}

	return bf
}

// WithTimeout sets the timeout for waiting on the backup to complete.
func WithTimeout(timeout time.Duration) func(*BackupFeature) {
	return func(bf *BackupFeature) {
		bf.timeout = timeout
	}
}

// WithInterval sets the interval for waiting on the backup to complete.
func WithInterval(interval time.Duration) func(*BackupFeature) {
	return func(bf *BackupFeature) {
		bf.interval = interval
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
		assert.NoError(t, err, "failed to create resources client")
		assert.NoError(t, r.Create(ctx, f.backup), "failed to create backup")
		err = wait.For(
			conditions.New(r).ResourceMatch(
				f.backup,
				func(object k8s.Object) bool {
					b, ok := object.(*cnpgv1.Backup)
					if !ok {
						return false
					}

					return b.Status.Phase == cnpgv1.BackupPhaseCompleted
				},
			),
			wait.WithTimeout(f.timeout),
			wait.WithInterval(f.interval),
		)
		assert.NoError(t, err, "backup not completed")

		return ctx
	}
}

// Teardown cleans up resources after the test is run.
func (f *BackupFeature) Teardown() types.StepFunc {
	return f.teardown
}
