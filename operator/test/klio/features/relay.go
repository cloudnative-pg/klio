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
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	"github.com/cloudnative-pg/klio/operator/test/klio/kopiaexec"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
)

// relayPollInterval is how often the relay feature looks at the tier2
// snapshots while a relay runs. A backup copied back by the relay stays on
// tier2 only until the next retention sweep deletes it, so the feature must
// look more often than the sweep runs.
const relayPollInterval = time.Second

// RelayFeature verifies the relay to tier2 with a tier2 retention tighter
// than tier1's: the relay must not copy back the backups that the tier2
// sweep removed, it must still complete a backup that tier2 holds only in
// part, and tier1 retention must still delete the backups that tier2 no
// longer holds.
type RelayFeature struct {
	*RetentionFeature
}

// NewRelayFeature creates a new RelayFeature with the given configuration.
// The cluster must relay backups to tier2, keep two backups on tier1 and one
// on tier2, and config.Backups must hold three backups.
func NewRelayFeature(config RetentionFeatureConfig) *RelayFeature {
	return &RelayFeature{RetentionFeature: NewRetentionFeature(config)}
}

// Run executes the relay feature test.
//
// The first two backups are relayed to tier2, whose retention then removes
// the first one, still kept on tier1. The second one then loses its pgdata
// snapshot on tier2. While the third backup is taken and relayed, the first
// one must never show up on tier2 again. Once the third is on tier2, tier1
// must delete the first backup, although tier2 no longer holds it, and each
// tier must hold what its retention keeps: the relay must have completed
// the second backup, as the sweep only deletes complete backups.
func (f *RelayFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		require.Len(t, f.backups, 3, "the relay feature needs exactly three backups")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		podName := f.klioServer.Name + klioPodSuffix
		first, second, third := f.backups[0], f.backups[1], f.backups[2]

		takeBackup(ctx, t, r, first, f.backupTimeout, f.checkInterval)
		firstName := first.Status.BackupID
		require.NotEmpty(t, firstName, "first backup has no backup ID in its status")

		takeBackup(ctx, t, r, second, f.backupTimeout, f.checkInterval)

		t.Logf("Waiting for tier2 retention to remove backup %q", firstName)
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				contents, err := tier2BackupContents(ctx, r, f.namespace, podName)
				if err != nil {
					return false, nil //nolint:nilerr // keep retrying on transient failures
				}
				if _, ok := contents[firstName]; ok {
					return false, nil
				}

				return checkBackupCounts(r, f.namespace, f.klioServer.Name, f.tier1Keep, f.tier2Keep)(ctx)
			},
			wait.WithTimeout(f.convergeTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "tier2 retention did not remove backup %q", firstName)

		secondName := second.Status.BackupID
		require.NotEmpty(t, secondName, "second backup has no backup ID in its status")
		deleted, err := deleteBackupPart(
			ctx, r, f.namespace, podName, kopiaexec.Tier2RWConfigPattern, secondName, "pgdata")
		require.NoError(t, err, "failed to delete the pgdata snapshot of backup %q from tier2", secondName)
		require.NotEmpty(t, deleted, "no pgdata snapshot of backup %q found on tier2", secondName)
		t.Logf("Deleted pgdata snapshots %v of backup %q from tier2", deleted, secondName)

		t.Logf("Creating backup %s", third.Name)
		require.NoError(t, r.Create(ctx, third), "failed to create backup %s", third.Name)

		t.Logf("Checking the relay of backup %s does not copy back %q", third.Name, firstName)
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				contents, err := tier2BackupContents(ctx, r, f.namespace, podName)
				if err != nil {
					return false, nil //nolint:nilerr // keep retrying on transient failures
				}
				if parts, ok := contents[firstName]; ok {
					return false, fmt.Errorf("%w: %q has snapshots %v", errBackupCopiedBack, firstName, parts)
				}

				completed, err := machineryConditions.BackupIsCompleted(r, third)(ctx)
				if err != nil || !completed {
					return false, err
				}

				return isRelayed(contents[third.Status.BackupID]), nil
			},
			wait.WithTimeout(f.backupTimeout+f.convergeTimeout),
			wait.WithInterval(relayPollInterval),
		)
		require.NoError(t, err, "backup %s was not relayed to tier2 alone", third.Name)

		t.Logf("Waiting for tier1 to converge to %d backup(s) and tier2 to %d backup(s)", f.tier1Keep, f.tier2Keep)
		err = wait.For(
			checkBackupCounts(r, f.namespace, f.klioServer.Name, f.tier1Keep, f.tier2Keep),
			wait.WithTimeout(f.convergeTimeout),
			wait.WithInterval(f.checkInterval),
		)
		require.NoError(t, err, "backup counts did not converge to tier1=%d tier2=%d", f.tier1Keep, f.tier2Keep)

		contents, err := tier2BackupContents(ctx, r, f.namespace, podName)
		require.NoError(t, err, "failed to list the tier2 snapshots")
		require.Len(t, contents, 1, "tier2 holds snapshots of backups that its retention does not keep")
		require.True(t, isRelayed(contents[third.Status.BackupID]),
			"backup %q is not complete on tier2", third.Status.BackupID)

		t.Log("Relay test completed")

		return ctx
	}
}

// errBackupCopiedBack is returned when the relay copies back a backup that
// tier2 retention already removed.
var errBackupCopiedBack = errors.New("the relay copied back a backup that tier2 retention removed")

// isRelayed reports whether contents, the content types of the snapshots of
// a backup without tablespaces, make a complete backup.
func isRelayed(contents []string) bool {
	for _, part := range []string{"pgdata", "controldata", "metadata"} {
		if !slices.Contains(contents, part) {
			return false
		}
	}

	return true
}

// tier2BackupContents returns, for each backup with a snapshot on tier2, the
// content types of its snapshots, listed with a direct Kopia connection from
// the Klio server pod.
func tier2BackupContents(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName string,
) (map[string][]string, error) {
	configFile, err := kopiaexec.ConfigFile(
		ctx, r, namespace, podName, serverContainerName, kopiaexec.Tier2RWConfigPattern)
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	cmd := []string{
		"kopia", "snapshot", "list", "--all", "--json",
		"--disable-file-logging", "--config-file=" + configFile,
	}
	if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, cmd, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("failed to list snapshots in pod %q: %w: %s", podName, err, stderr.String())
	}

	var manifests []struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &manifests); err != nil {
		return nil, fmt.Errorf("failed to decode the snapshot list: %w", err)
	}

	// Kopia prefixes the keys of the user-defined tags with "tag:".
	contents := make(map[string][]string)
	for _, m := range manifests {
		name := m.Tags["tag:"+backupNameTagName]
		contents[name] = append(contents[name], m.Tags["tag:"+backupContentTagName])
	}

	return contents, nil
}
