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

package retention

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// hostBackupName identifies one backup attempt on one cluster.
type hostBackupName struct {
	Host string
	Name string
}

// sweepOrphans finds and deletes, on tc's tier, every backup attempt that
// never produced a metadata snapshot (an interrupted upload) and for which
// a later, completed backup exists on the same cluster -- proof the
// interrupted attempt was abandoned rather than still in flight. It returns
// how many it deleted.
func (s *Sweeper) sweepOrphans(ctx context.Context, tc tierConfig, hostnames []string) (int, error) {
	contextLogger := log.FromContext(ctx)

	// Unfiltered: orphan detection needs every content type (tablespace,
	// pgdata, controldata, metadata), not just the metadata-tagged entries
	// listClusterHostnames looked at.
	entries, err := tc.kopiaClient.ListSnapshots(ctx, nil, contextLogger.Info)
	if err != nil {
		return 0, fmt.Errorf("while executing Kopia command: %w", err)
	}

	orphans := orphanBackupNames(entries, hostnames)

	deleted := 0
	for _, o := range orphans {
		if err := tc.backupClient.DeleteBackup(ctx, o.Host, o.Name); err != nil {
			contextLogger.Error(err, "Error while deleting orphaned backup", "cluster", o.Host, "backup", o.Name)
			continue
		}
		deleted++
	}

	return deleted, nil
}

// backupGroup tracks, for one (host, backup-name) pair, whether a metadata
// snapshot was seen and the earliest StartTime among its snapshots.
type backupGroup struct {
	hasMetadata bool
	startTime   string
}

// orphanBackupNames groups entries by (host, backup-name tag), restricted
// to allowedHosts, and returns the groups that have no metadata member but
// do have a later, completed (has-metadata) group on the same host.
//
// allowedHosts is meant to be the set of clusters known to have at least
// one completed backup (what listClusterHostnames returns): a cluster with
// none can never satisfy the "completed backup afterward" condition, so
// restricting to it up front is a correct, cheap prefilter, not just an
// optimization.
//
// This assumes at most one backup in flight per cluster at a time
// (consistent with how CNPG schedules backups); it does not try to
// disambiguate concurrent backup attempts on the same cluster.
func orphanBackupNames(entries []kopia.Manifest, allowedHosts []string) []hostBackupName {
	groups := groupBackupsByHostName(entries, allowedHosts)
	latestCompleted := latestCompletedStartTimePerHost(groups)

	var orphans []hostBackupName
	for key, g := range groups {
		if g.hasMetadata {
			continue
		}

		completedAfter, ok := latestCompleted[key.Host]
		if !ok {
			// No completed backup at all for this host: could still be in
			// flight, leave it alone.
			continue
		}

		if completedAfter > g.startTime {
			orphans = append(orphans, key)
		}
	}

	return orphans
}

// groupBackupsByHostName groups entries restricted to allowedHosts by
// (host, backup-name tag), tracking per group whether a metadata snapshot
// was seen and the earliest StartTime among its snapshots.
func groupBackupsByHostName(entries []kopia.Manifest, allowedHosts []string) map[hostBackupName]*backupGroup {
	allowed := make(map[string]struct{}, len(allowedHosts))
	for _, h := range allowedHosts {
		allowed[h] = struct{}{}
	}

	groups := make(map[hostBackupName]*backupGroup)

	for _, entry := range entries {
		host := entry.Source.Host
		if _, ok := allowed[host]; !ok {
			continue
		}

		name := entry.Tags[klioclient.BackupNameTagName]
		if name == "" {
			continue
		}

		key := hostBackupName{Host: host, Name: name}
		g, ok := groups[key]
		if !ok {
			g = &backupGroup{startTime: entry.StartTime}
			groups[key] = g
		}

		if entry.Tags[klioclient.BackupContentTagName] == "metadata" {
			g.hasMetadata = true
		}

		if g.startTime == "" || entry.StartTime < g.startTime {
			g.startTime = entry.StartTime
		}
	}

	return groups
}

// latestCompletedStartTimePerHost returns, per host, the StartTime of its
// latest completed (has-metadata) backup group.
func latestCompletedStartTimePerHost(groups map[hostBackupName]*backupGroup) map[string]string {
	latestCompleted := make(map[string]string)

	for key, g := range groups {
		if !g.hasMetadata {
			continue
		}
		if cur, ok := latestCompleted[key.Host]; !ok || g.startTime > cur {
			latestCompleted[key.Host] = g.startTime
		}
	}

	return latestCompleted
}
