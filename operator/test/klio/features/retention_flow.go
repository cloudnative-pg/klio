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
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"

	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
)

// retentionFlowParams carries what the tier-agnostic retention flow needs to
// exercise one tier's retention: automatic deletion of the oldest backup once
// newer backups push it outside the policy, followed by on-demand deletion via
// `klio retention apply`. Tier1 and tier2 differ only in the tier annotation,
// the label used in logs, and which tier's policy setRetentionLatest edits.
type retentionFlowParams struct {
	backups                 []*cnpgv1.Backup
	serverName              string
	namespace               string
	clusterName             string
	keepLatest              int
	tierLabel               string
	tierAnnotation          string
	pluginConfigurationName string
	backupTimeout           time.Duration
	retentionTimeout        time.Duration
	checkInterval           time.Duration
	// setRetentionLatest tightens this tier's policy to keep `latest` backups.
	setRetentionLatest func(ctx context.Context, t *testing.T, r *resources.Resources, latest int)
	// onFirstBackup, if set, runs once right after the first backup reaches the
	// tier. Tier2 uses it to record a WAL directory baseline.
	onFirstBackup func(ctx context.Context)
}

// verifyRetentionAndOnDemandApply runs the tier-agnostic retention flow shared
// by the tier1 and tier2 retention features. It creates more backups than the
// policy keeps and verifies the tier ends up with exactly keepLatest backups
// with the oldest deleted, then tightens the policy to a single backup, applies
// it on demand with `klio retention apply`, and verifies only the newest
// remains. This exercises the full server-side path: PluginConfiguration CR ->
// operator -> klio-plugin config -> CloseBackup GRPC -> NATS queue -> backup
// consumer -> Klio-managed retention.
func verifyRetentionAndOnDemandApply(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	p retentionFlowParams,
) {
	t.Helper()

	// Name of the first (oldest) backup. Retention must delete it once newer
	// backups push it outside the policy.
	var oldestBackupName string

	for i, backup := range p.backups {
		t.Logf("Creating backup %d/%d: %s", i+1, len(p.backups), backup.Name)
		require.NoError(t, r.Create(ctx, backup), "failed to create backup %s", backup.Name)

		err := wait.For(
			machineryConditions.BackupIsCompleted(r, backup),
			wait.WithTimeout(p.backupTimeout),
			wait.WithInterval(p.checkInterval),
		)
		require.NoError(t, err, "backup %s did not complete", backup.Name)
		t.Logf("Backup %s completed successfully", backup.Name)

		// After more backups than the policy keeps, the older ones are deleted.
		expectedBackups := min(i+1, p.keepLatest)
		err = wait.For(
			checkTierHasBackups(r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation, expectedBackups),
			wait.WithTimeout(p.retentionTimeout),
			wait.WithInterval(p.checkInterval),
		)
		require.NoError(t, err, "%s retention not applied after backup %d", p.tierLabel, i+1)
		t.Logf("%s has expected %d backup(s) after backup %d", p.tierLabel, expectedBackups, i+1)

		if i == 0 {
			names, listErr := listTierBackupNames(
				ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
			require.NoError(t, listErr, "failed to list %s backups after the first backup", p.tierLabel)
			require.Len(t, names, 1, "expected exactly one %s backup after the first backup", p.tierLabel)
			oldestBackupName = names[0]
			t.Logf("Oldest %s backup recorded: %s", p.tierLabel, oldestBackupName)

			if p.onFirstBackup != nil {
				p.onFirstBackup(ctx)
			}
		}
	}

	// Automatic retention: exactly keepLatest backups remain and the oldest was
	// the one the retention manager deleted (the newest survive).
	t.Logf("Retention verification: %s should have exactly %d backup(s)", p.tierLabel, p.keepLatest)
	err := wait.For(
		checkTierHasBackups(r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation, p.keepLatest),
		wait.WithTimeout(p.retentionTimeout),
		wait.WithInterval(p.checkInterval),
	)
	require.NoError(t, err, "%s backup count verification failed", p.tierLabel)

	survivingNames, err := listTierBackupNames(
		ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
	require.NoError(t, err, "could not list surviving %s backups", p.tierLabel)
	require.NotContains(t, survivingNames, oldestBackupName,
		"the oldest backup should have been deleted by the %s retention manager", p.tierLabel)
	t.Logf("PASSED: %s has exactly %d backup(s) and the oldest (%s) was deleted",
		p.tierLabel, p.keepLatest, oldestBackupName)

	// On-demand retention: tighten the policy to keep a single backup and apply
	// it immediately with `klio retention apply`, without taking a new backup.
	// Only the newest backup must remain afterwards.
	newestBackupName := survivingNames[0]
	t.Logf("On-demand retention: tightening %s retention to 1 and running `klio retention apply`", p.tierLabel)
	p.setRetentionLatest(ctx, t, r, 1)

	instancePodName := p.backups[len(p.backups)-1].Status.InstanceID.PodName
	require.NotEmpty(t, instancePodName, "backup instance pod name should be set")

	// The `klio retention apply` command sends the policy from the pod's mounted
	// config, which the operator updates asynchronously after the
	// PluginConfiguration change. Re-running the (idempotent) command until
	// exactly one backup remains absorbs both the config propagation and the
	// asynchronous consumer processing.
	err = wait.For(
		func(ctx context.Context) (bool, error) {
			if applyErr := runRetentionApply(ctx, r, p.namespace, instancePodName); applyErr != nil {
				t.Logf("klio retention apply not ready yet: %v", applyErr)

				return false, nil
			}

			names, listErr := listTierBackupNames(
				ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
			if listErr != nil {
				return false, nil //nolint:nilerr
			}

			return len(names) == 1, nil
		},
		wait.WithTimeout(p.retentionTimeout),
		wait.WithInterval(p.checkInterval),
	)
	require.NoError(t, err, "on-demand %s retention did not converge to a single backup", p.tierLabel)

	finalNames, err := listTierBackupNames(
		ctx, r, p.namespace, p.serverName, p.clusterName, p.tierAnnotation)
	require.NoError(t, err, "could not list surviving %s backups", p.tierLabel)
	require.Equal(t, []string{newestBackupName}, finalNames,
		"only the newest backup should remain after `klio retention apply`")
	t.Logf("PASSED: only the newest %s backup (%s) remains after apply", p.tierLabel, newestBackupName)
}
