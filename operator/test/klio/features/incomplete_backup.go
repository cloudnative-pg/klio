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
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	klioConditions "github.com/cloudnative-pg/klio/operator/test/klio/conditions"
	"github.com/cloudnative-pg/klio/operator/test/klio/kopiaexec"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
)

const (
	// backupNameTagName is the Kopia tag holding a snapshot's backup name.
	backupNameTagName = "klio.io/tag"
	// backupContentTagName is the Kopia tag holding a snapshot's content type.
	backupContentTagName = "klio.io/content"
)

// IncompleteBackupFeature verifies that a backup missing one of its
// snapshots is not listed, is reported by `klio admin list-backups
// --incomplete` with the right status, survives the retention sweep, can
// be deleted by hand when it can never complete and is rebuilt by the
// relay when it can; and that tier1 WAL retention moves past a deleted
// backup.
type IncompleteBackupFeature struct {
	name          string
	setup         types.StepFunc
	teardown      types.StepFunc
	backups       []*cnpgv1.Backup
	klioServer    *kliov1alpha1.Server
	namespace     string
	clusterName   string
	backupTimeout time.Duration
	waitTimeout   time.Duration
	checkInterval time.Duration
}

// IncompleteBackupFeatureConfig holds the configuration for creating an
// incomplete backup feature test.
type IncompleteBackupFeatureConfig struct {
	// Name of the feature test.
	Name string
	// Setup function to initialize test resources.
	Setup types.StepFunc
	// Teardown function to clean up test resources.
	Teardown types.StepFunc
	// Backups are the three backup resources to be created, in order. The
	// cluster must relay backups to tier2, keep only the latest backup on
	// tier1 and at least three on tier2.
	Backups []*cnpgv1.Backup
	// KlioServer is the Klio server resource.
	KlioServer *kliov1alpha1.Server
	// Namespace is the namespace where resources are created.
	Namespace string
	// ClusterName is the name of the CNPG cluster (used for WAL directory lookup).
	ClusterName string
	// BackupTimeout is the timeout for each backup to complete (defaults to 2 minutes).
	BackupTimeout time.Duration
	// WaitTimeout is the timeout for relay and retention to act (defaults to 5 minutes).
	WaitTimeout time.Duration
	// CheckInterval is the interval for checking status (defaults to 10 seconds).
	CheckInterval time.Duration
}

// NewIncompleteBackupFeature creates a new IncompleteBackupFeature with the
// given configuration.
func NewIncompleteBackupFeature(config IncompleteBackupFeatureConfig) *IncompleteBackupFeature {
	if config.BackupTimeout <= 0 {
		config.BackupTimeout = 2 * time.Minute
	}
	if config.WaitTimeout <= 0 {
		config.WaitTimeout = 5 * time.Minute
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 10 * time.Second
	}

	return &IncompleteBackupFeature{
		name:          config.Name,
		setup:         config.Setup,
		teardown:      config.Teardown,
		backups:       config.Backups,
		klioServer:    config.KlioServer,
		namespace:     config.Namespace,
		clusterName:   config.ClusterName,
		backupTimeout: config.BackupTimeout,
		waitTimeout:   config.WaitTimeout,
		checkInterval: config.CheckInterval,
	}
}

// Name returns the name of the feature.
func (f *IncompleteBackupFeature) Name() string {
	return f.name
}

// Setup initializes the feature test.
func (f *IncompleteBackupFeature) Setup() types.StepFunc {
	return f.setup
}

// Teardown cleans up resources after the test is run.
func (f *IncompleteBackupFeature) Teardown() types.StepFunc {
	return f.teardown
}

// Run executes the incomplete backup feature test.
//
// The first backup is relayed to tier2, then its pgdata snapshot is deleted
// from both tiers, so no later relay can copy it back: it must disappear
// from the backup list and be reported as unrecoverable on both tiers. The
// second backup loses its pgdata snapshot on tier2 only: it must be
// reported as recoverable. Both must survive the retention sweep. The
// first is then deleted with `klio admin delete-backup`, and the relay of
// the third backup must rebuild the second one on tier2. Once the third
// backup is complete, tier1 holds only it, and tier1 WAL retention must
// drop the WALs the earlier backups needed.
func (f *IncompleteBackupFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		require.Len(t, f.backups, 3, "the incomplete backup feature needs exactly three backups")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		podName := f.klioServer.Name + klioPodSuffix
		first, second, third := f.backups[0], f.backups[1], f.backups[2]

		f.takeBackup(ctx, t, r, first)
		t.Log("Waiting for the first backup to be relayed to tier2")
		err = wait.For(
			klioConditions.Tier2BackupCountEquals(r, f.namespace, podName, serverContainerName, 1),
			wait.WithTimeout(f.waitTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "first backup was not relayed to tier2")

		firstName := first.Status.BackupID
		require.NotEmpty(t, firstName, "first backup has no backup ID in its status")
		f.deletePgData(ctx, t, r, podName, kopiaexec.Tier1ConfigPattern, firstName)
		f.deletePgData(ctx, t, r, podName, kopiaexec.Tier2RWConfigPattern, firstName)

		t.Log("Waiting for the incomplete backup to disappear from the backup list")
		err = wait.For(
			klioConditions.BackupCountEquals(r, f.namespace, podName, serverContainerName, 0),
			wait.WithTimeout(f.waitTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "incomplete backup %q is still listed", firstName)

		f.takeBackup(ctx, t, r, second)
		t.Log("Waiting for the second backup to be listed on both tiers")
		err = wait.For(
			checkBackupCounts(r, f.namespace, f.klioServer.Name, 1, 1),
			wait.WithTimeout(f.waitTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "second backup is not listed on both tiers")

		secondName := second.Status.BackupID
		require.NotEmpty(t, secondName, "second backup has no backup ID in its status")
		f.deletePgData(ctx, t, r, podName, kopiaexec.Tier2RWConfigPattern, secondName)

		expected := []incompleteBackup{
			f.incompleteBackup(firstName, "tier1", "UNRECOVERABLE"),
			f.incompleteBackup(firstName, "tier2", "UNRECOVERABLE"),
			f.incompleteBackup(secondName, "tier2", "RECOVERABLE"),
		}
		t.Log("Waiting for the damaged backups to be reported with their status")
		err = wait.For(
			f.incompleteBackupsAre(r, podName, expected),
			wait.WithTimeout(f.waitTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "damaged backups were not reported as expected")

		// The sweep runs every minute: give it two to show it keeps the
		// incomplete backups, whatever their status.
		t.Log("Checking the incomplete backups survive the retention sweep")
		time.Sleep(2 * time.Minute)
		ok, err := f.incompleteBackupsAre(r, podName, expected)(ctx)
		require.NoError(t, err)
		require.True(t, ok, "the retention sweep changed the incomplete backups")

		t.Logf("Deleting the unrecoverable backup %q from both tiers", firstName)
		var stdout, stderr bytes.Buffer
		deleteCmd := []string{
			"klio", "admin", "delete-backup", firstName, "--cluster", f.clusterName, "--tier1", "--tier2",
		}
		err = r.ExecInPod(ctx, f.namespace, podName, serverContainerName, deleteCmd, &stdout, &stderr)
		require.NoError(t, err, "failed to delete backup %q: %s", firstName, stderr.String())

		err = wait.For(
			f.incompleteBackupsAre(r, podName, expected[2:]),
			wait.WithTimeout(f.waitTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "unrecoverable backup %q is still reported", firstName)

		f.takeBackup(ctx, t, r, third)
		t.Log("Waiting for the relay of the third backup to rebuild the second one on tier2")
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				if ok, err := f.incompleteBackupsAre(r, podName, nil)(ctx); err != nil || !ok {
					return false, err
				}

				return checkBackupCounts(r, f.namespace, f.klioServer.Name, 1, 2)(ctx)
			},
			wait.WithTimeout(f.waitTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "backup %q was not rebuilt on tier2", secondName)

		t.Logf("Waiting for the tier1 WAL floor to reach %q", third.Status.BeginWal)
		var walFiles []string
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				walFiles, err = listTier1WALFiles(ctx, r, f.namespace, podName, f.clusterName)
				if err != nil {
					return false, err
				}

				return len(walsOlderThan(walFiles, third.Status.BeginWal)) == 0, nil
			},
			wait.WithTimeout(f.waitTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "tier1 WAL retention did not prune WALs older than %q; remaining: %v",
			third.Status.BeginWal, walsOlderThan(walFiles, third.Status.BeginWal))

		t.Log("Incomplete backup test completed")

		return ctx
	}
}

// incompleteBackup returns the report row of a backup of the feature's
// cluster whose pgdata snapshot was deleted on the given tier.
func (f *IncompleteBackupFeature) incompleteBackup(backupName, tier, status string) incompleteBackup {
	return incompleteBackup{
		ClusterName:     f.clusterName,
		BackupName:      backupName,
		Tier:            tier,
		Status:          status,
		MetadataPresent: true,
		MissingParts:    []string{"pgdata"},
	}
}

// incompleteBackupsAre checks that `klio admin list-backups --incomplete`
// reports exactly expected. It returns (false, nil) on transient errors,
// so the wait keeps retrying.
func (f *IncompleteBackupFeature) incompleteBackupsAre(
	r *resources.Resources,
	podName string,
	expected []incompleteBackup,
) func(ctx context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		backups, err := listIncompleteBackups(ctx, r, f.namespace, podName)
		if err != nil {
			return false, nil //nolint:nilerr // keep retrying on transient failures
		}

		return len(backups) == len(expected) && reflect.DeepEqual(backups, expected), nil
	}
}

// deletePgData deletes the pgdata snapshot of the named backup with the
// given Kopia configuration, and checks the backup is left incomplete, not
// gone.
func (f *IncompleteBackupFeature) deletePgData(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	podName, configPattern, backupName string,
) {
	t.Helper()

	deleted, err := deleteBackupPart(ctx, r, f.namespace, podName, configPattern, backupName, "pgdata")
	require.NoError(t, err, "failed to delete the pgdata snapshot of backup %q", backupName)
	require.NotEmpty(t, deleted, "no pgdata snapshot of backup %q found with %s", backupName, configPattern)
	t.Logf("Deleted pgdata snapshots %v of backup %q with %s", deleted, backupName, configPattern)

	metadata, err := listBackupParts(ctx, r, f.namespace, podName, configPattern, backupName, "metadata")
	require.NoError(t, err, "failed to list the metadata snapshot of backup %q", backupName)
	require.NotEmpty(t, metadata, "no metadata snapshot of backup %q found with %s", backupName, configPattern)
}

// incompleteBackup is the subset of the `klio admin list-backups
// --incomplete` JSON output this feature checks.
type incompleteBackup struct {
	ClusterName     string   `json:"clusterName"`
	BackupName      string   `json:"backupName"`
	Tier            string   `json:"tier"`
	Status          string   `json:"status"`
	MetadataPresent bool     `json:"metadataPresent"`
	MissingParts    []string `json:"missingParts"`
}

// listIncompleteBackups runs `klio admin list-backups --incomplete` in the
// Klio server pod.
func listIncompleteBackups(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName string,
) ([]incompleteBackup, error) {
	var stdout, stderr bytes.Buffer
	cmd := []string{"klio", "admin", "list-backups", "--incomplete"}
	if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, cmd, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("failed to list incomplete backups in pod %q: %w: %s", podName, err, stderr.String())
	}

	var backups []incompleteBackup
	if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
		return nil, fmt.Errorf("failed to decode the incomplete backup list: %w", err)
	}

	return backups, nil
}

// takeBackup creates backup and waits for it to complete.
func (f *IncompleteBackupFeature) takeBackup(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	backup *cnpgv1.Backup,
) {
	t.Helper()

	t.Logf("Creating backup %s", backup.Name)
	require.NoError(t, r.Create(ctx, backup), "failed to create backup %s", backup.Name)

	err := wait.For(
		machineryConditions.BackupIsCompleted(r, backup),
		wait.WithTimeout(f.backupTimeout),
		wait.WithInterval(f.checkInterval),
	)
	require.NoError(t, err, "backup %s did not complete", backup.Name)
	t.Logf("Backup %s completed with ID %q", backup.Name, backup.Status.BackupID)
}

// deleteBackupPart deletes, with a direct Kopia connection from the Klio
// server pod, the snapshots of one content type of a backup, and returns
// their IDs.
func deleteBackupPart(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName, configPattern, backupName, content string,
) ([]string, error) {
	configFile, err := kopiaexec.ConfigFile(ctx, r, namespace, podName, serverContainerName, configPattern)
	if err != nil {
		return nil, err
	}

	ids, err := listBackupParts(ctx, r, namespace, podName, configPattern, backupName, content)
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		var stdout, stderr bytes.Buffer
		cmd := []string{
			"kopia", "snapshot", "delete", id, "--delete",
			"--disable-file-logging", "--config-file=" + configFile,
		}
		if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, cmd, &stdout, &stderr); err != nil {
			return nil, fmt.Errorf("failed to delete snapshot %q: %w: %s", id, err, stderr.String())
		}
	}

	return ids, nil
}

// listBackupParts returns the IDs of the snapshots of one content type of a
// backup, listed with a direct Kopia connection from the Klio server pod.
func listBackupParts(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName, configPattern, backupName, content string,
) ([]string, error) {
	configFile, err := kopiaexec.ConfigFile(ctx, r, namespace, podName, serverContainerName, configPattern)
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	cmd := []string{
		"kopia", "snapshot", "list", "--all", "--json",
		"--disable-file-logging", "--config-file=" + configFile,
		"--tags=" + backupNameTagName + ":" + backupName,
		"--tags=" + backupContentTagName + ":" + content,
	}
	if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, cmd, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("failed to list snapshots in pod %q: %w: %s", podName, err, stderr.String())
	}

	var manifests []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &manifests); err != nil {
		return nil, fmt.Errorf("failed to decode the snapshot list: %w", err)
	}

	ids := make([]string, 0, len(manifests))
	for _, m := range manifests {
		ids = append(ids, m.ID)
	}

	return ids, nil
}
