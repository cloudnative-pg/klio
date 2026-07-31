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
	"fmt"
	"strings"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
)

// maintenanceClusterName is the CNPG cluster name created by newBackupFeature.
const maintenanceClusterName = "test-cluster"

// listTier1WALFiles returns the WAL segment file names stored in tier1 for a
// cluster, sorted ascending. Partial files are excluded.
//
// WAL files live at /data/wal/{clusterName}/{16-char-prefix}/{24-char-name} on
// the server pod; 'find' is unavailable in the minimal container, so we rely on
// shell globbing.
func listTier1WALFiles(
	ctx context.Context,
	r *resources.Resources,
	namespace, podName, clusterName string,
) []string {
	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"sh", "-c",
		fmt.Sprintf("ls /data/wal/%s/*/0000* 2>/dev/null | sort", clusterName),
	}

	// ls exits non-zero when nothing matches, which is fine: we return an empty
	// list in that case.
	_ = r.ExecInPod(ctx, namespace, podName, serverContainerName, listCmd, &stdout, &stderr)

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil
	}

	var walFiles []string
	for line := range strings.SplitSeq(output, "\n") {
		parts := strings.Split(line, "/")
		name := parts[len(parts)-1]
		if strings.HasSuffix(name, ".partial") {
			continue
		}
		walFiles = append(walFiles, name)
	}

	return walFiles
}

// walsOlderThan returns the entries of walFiles that are strictly older than
// boundary. WAL segment names are fixed-width hex, so a lexicographic
// comparison matches WAL ordering.
func walsOlderThan(walFiles []string, boundary string) []string {
	var older []string
	for _, w := range walFiles {
		if w < boundary {
			older = append(older, w)
		}
	}

	return older
}

// assertServerSideMaintenanceRan verifies that, for a tier1-only deployment,
// the Klio server applies tier1 WAL retention after a backup completes.
//
// Post-backup maintenance is performed by the backup queue consumer, which runs
// whenever tier1 is enabled (it does not require tier2). Rather than trusting a
// log line, this asserts the on-disk effect of retention: once maintenance has
// run, no tier1 WAL segment older than the backup's begin WAL may survive,
// because the only remaining backup requires nothing older than that.
func assertServerSideMaintenanceRan(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	backup *cnpgv1.Backup,
) {
	t.Helper()

	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	serverPodName := klioServerName + "-klio-0"

	// Refresh the backup to read the begin WAL recorded in its status; this is
	// the first-required WAL that tier1 retention enforces for the only backup.
	var completed cnpgv1.Backup
	require.NoError(t, r.Get(ctx, backup.Name, backup.Namespace, &completed),
		"failed to get completed backup")
	boundary := completed.Status.BeginWal
	require.NotEmpty(t, boundary, "completed backup has no begin WAL in its status")

	initial := listTier1WALFiles(ctx, r, backup.Namespace, serverPodName, maintenanceClusterName)
	t.Logf("Tier1 WAL files before maintenance settled: %d (%v), begin WAL %q", len(initial), initial, boundary)

	// Maintenance is asynchronous: the consumer processes the backup after it is
	// closed. Poll until every WAL older than the begin WAL has been removed.
	t.Log("Waiting for server-side tier1 WAL retention to remove WALs older than the backup begin WAL")
	err = wait.For(
		func(ctx context.Context) (bool, error) {
			walFiles := listTier1WALFiles(ctx, r, backup.Namespace, serverPodName, maintenanceClusterName)
			return len(walsOlderThan(walFiles, boundary)) == 0, nil
		},
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)

	final := listTier1WALFiles(ctx, r, backup.Namespace, serverPodName, maintenanceClusterName)
	require.NoError(t, err,
		"server-side tier1 maintenance did not prune WALs older than begin WAL %q; remaining older WALs: %v",
		boundary, walsOlderThan(final, boundary))

	// The cluster keeps archiving, and the begin WAL itself is retained, so the
	// repository must not be empty: an empty result would mean we measured the
	// wrong path rather than a successful retention.
	require.NotEmpty(t, final, "no tier1 WAL files found for cluster %q after maintenance", maintenanceClusterName)

	t.Logf("Verified server-side tier1 WAL retention: %d WAL files remain, all >= begin WAL %q", len(final), boundary)
}

// Tier1ServerSideMaintenance returns a Feature that verifies tier1 maintenance
// runs server-side after a backup on a tier1-only deployment.
func Tier1ServerSideMaintenance(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature(
		"Tier1ServerSideMaintenance", cnpgv1.BackupTargetPrimary, 1, namespace, assertServerSideMaintenanceRan)
}
