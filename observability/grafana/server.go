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

package main

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/units"
)

// snapshotCluster wraps a Kopia base-snapshot selector in a label_replace that
// derives a `cluster` label from the snapshot_source attribute (formatted
// `userName@clusterName:path`). The per-source snapshot gauges carry
// snapshot_source and tier but no cluster_name, so this lets them be grouped
// per PostgreSQL cluster instead of folding every cluster on a server into one
// value.
func snapshotCluster(selector string) string {
	return fmt.Sprintf(`label_replace(%s, "cluster", "$1", "snapshot_source", "[^@]*@([^:]+):.*")`, selector)
}

// serverLabel wraps a Klio server selector in a label_replace that derives a
// `server` label, the Server name, from the pod name (`<server>-klio-<ordinal>`),
// so the series can be grouped and named by server namespace and name.
func serverLabel(selector string) string {
	return fmt.Sprintf(`label_replace(%s, "server", "$1", "k8s_pod_name", "(.+)-klio-[0-9]+")`, selector)
}

// The server panels show the metrics of the Klio server (`klio_server_*`).
// Panels shown on both dashboards take their series matcher.

// retainedBackupsPanel counts the retained backups, naming the server that
// retains them.
func retainedBackupsPanel(sc scope) *statPanelBuilder {
	return statPanel("Retained backups", units.Number,
		query(fmt.Sprintf("sum by (cluster_name, tier, k8s_namespace_name, server) (%s)",
			serverLabel(sc.series("klio_server_backup_backups"))),
			"{{cluster_name}} {{tier}} ({{k8s_namespace_name}}/{{server}})"),
	).Decimals(0).
		Orientation(common.VizOrientationHorizontal).
		Description("Number of PostgreSQL backups currently retained per cluster and tier, with the Klio server " +
			"that retains them.")
}

// latestBackupAgePanel shows the age of the latest retained backup.
func latestBackupAgePanel(sc scope) *statPanelBuilder {
	return statPanel("Latest backup age (completion)", units.DurationInDaysHoursMinutesSeconds,
		query(
			fmt.Sprintf("time() - max by (cluster_name, tier) (%s)",
				sc.series("klio_server_backup_latest_backup_completion_time_seconds")),
			"{{cluster_name}} {{tier}}"),
	).Orientation(common.VizOrientationHorizontal).
		Description("Elapsed time since the most recently retained PostgreSQL backup completed, per cluster " +
			"and tier.")
}

// timeSinceLastWALPanel shows the time since the last WAL write.
func timeSinceLastWALPanel(sc scope) *timeseriesPanelBuilder {
	return timeseriesPanel("Time since last WAL written", units.DurationSeconds,
		query(fmt.Sprintf("time() - max by (cluster_name, tier) (%s)",
			sc.series("klio_server_wal_latest_written_time_seconds")), "{{cluster_name}} {{tier}}"),
	).Description("Elapsed time since the server last wrote a WAL file for each cluster and tier.")
}

// walBytesRatePanel plots the WAL write throughput.
func walBytesRatePanel(sc scope) *timeseriesPanelBuilder {
	return timeseriesPanel("WAL bytes written (rate)", units.BytesPerSecondIEC,
		query(
			fmt.Sprintf("sum by (cluster_name, tier) (%s)",
				sc.over("rate", "klio_server_wal_written_size_bytes_total", "$__rate_interval")),
			"{{cluster_name}} {{tier}}"),
	).Description("Rate of WAL bytes written by the server, split by cluster and storage tier.")
}

// walBytesTotalPanel plots the WAL bytes written since the server restarted.
func walBytesTotalPanel(sc scope) *timeseriesPanelBuilder {
	return timeseriesPanel("WAL bytes written (total)", units.BytesIEC,
		query(fmt.Sprintf("sum by (cluster_name, tier) (%s)", sc.series("klio_server_wal_written_size_bytes_total")),
			"{{cluster_name}} {{tier}}"),
	).Description("Total WAL bytes written by the server since it last restarted, split by cluster and " +
		"storage tier.")
}

// retainedBackupPanels returns the retention window panels.
func retainedBackupPanels() []sizedPanel {
	return []sizedPanel{
		sized(largePanelWidth, largePanelHeight, retainedBackupsPanel(servedScope)),
		sized(largePanelWidth, largePanelHeight,
			statPanel("Oldest restorable point", units.DatetimeISO,
				query(
					fmt.Sprintf("min by (cluster_name, tier) "+
						"(%s) * 1000", servedScope.series("klio_server_backup_oldest_backup_completion_time_seconds")),
					"{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Timestamp at which the oldest retained PostgreSQL backup completed, per cluster and "+
					"tier. It is the earliest point in time a restore can target, as long as all the WAL files "+
					"since the backup started are still retained.")),

		// Latest retained backup.
		sized(largePanelWidth, largePanelHeight, latestBackupAgePanel(servedScope)),
		sized(largePanelWidth, largePanelHeight,
			statPanel("Latest backup age (start)", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("time() - max by (cluster_name, tier) "+
					"(%s)", servedScope.series("klio_server_backup_latest_backup_start_time_seconds")), "{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the most recently retained PostgreSQL backup started, per cluster "+
					"and tier.")),

		sized(largePanelWidth, largePanelHeight, statPanel("Latest backup timeline", units.Number,
			query(fmt.Sprintf("max by (cluster_name, tier) (%s)",
				servedScope.series("klio_server_backup_latest_backup_timeline")), "{{cluster_name}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("PostgreSQL timeline of the latest retained backup, per cluster and tier. Differing "+
				"from the oldest timeline means the retention window spans a promotion or failover.")),

		sized(gridWidth, tablePanelHeight, lsnTablePanel("Latest backup LSN",
			lsnColumn{"start", fmt.Sprintf(
				"max by (cluster_name, tier) (%s)", servedScope.series("klio_server_backup_latest_backup_start_lsn_bytes"))},
			lsnColumn{"end", fmt.Sprintf(
				"max by (cluster_name, tier) (%s)", servedScope.series("klio_server_backup_latest_backup_end_lsn_bytes"))},
		).Description("Start and end LSN of the latest retained PostgreSQL backup, per cluster and tier.")),

		// Oldest retained backup, i.e. the edge of the retention window.
		sized(largePanelWidth, largePanelHeight,
			statPanel("Oldest backup age (completion)", units.DurationInDaysHoursMinutesSeconds,
				query(
					fmt.Sprintf("time() - min by (cluster_name, tier) "+
						"(%s)", servedScope.series("klio_server_backup_oldest_backup_completion_time_seconds")),
					"{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the oldest retained PostgreSQL backup completed, per cluster and "+
					"tier, reflecting each tier's effective retention horizon.")),

		sized(largePanelWidth, largePanelHeight,
			statPanel("Oldest backup age (start)", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("time() - min by (cluster_name, tier) "+
					"(%s)", servedScope.series("klio_server_backup_oldest_backup_start_time_seconds")), "{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the oldest retained PostgreSQL backup started, per cluster and tier, "+
					"reflecting each tier's effective retention horizon.")),

		sized(largePanelWidth, largePanelHeight, statPanel("Oldest backup timeline", units.Number,
			query(fmt.Sprintf("max by (cluster_name, tier) (%s)",
				servedScope.series("klio_server_backup_oldest_backup_timeline")), "{{cluster_name}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("PostgreSQL timeline of the oldest retained backup, per cluster and tier. Differing "+
				"from the latest timeline means the retention window spans a promotion or failover.")),

		sized(gridWidth, tablePanelHeight, lsnTablePanel("Oldest backup LSN",
			lsnColumn{"start", fmt.Sprintf(
				"max by (cluster_name, tier) (%s)", servedScope.series("klio_server_backup_oldest_backup_start_lsn_bytes"))},
			lsnColumn{"end", fmt.Sprintf(
				"max by (cluster_name, tier) (%s)", servedScope.series("klio_server_backup_oldest_backup_end_lsn_bytes"))},
		).Description("Start and end LSN of the oldest retained PostgreSQL backup, per cluster and tier.")),
	}
}

// walIngestPanels returns the WAL written by the server.
func walIngestPanels() []sizedPanel {
	return []sizedPanel{
		sized(largePanelWidth, mediumPanelHeight, timelinePanel("WAL timeline",
			query(fmt.Sprintf("max by (cluster_name, tier) (%s)",
				clusterScope.series("klio_server_wal_latest_written_timeline")),
				"{{cluster_name}} {{tier}}"),
		).Description("PostgreSQL timeline of the latest WAL written per cluster and tier")),

		sized(gridWidth, tablePanelHeight, lsnTablePanel("Latest written LSN",
			lsnColumn{"written", fmt.Sprintf(
				"max by (cluster_name, tier) (%s)", clusterScope.series("klio_server_wal_latest_written_lsn_bytes"))},
		).Description("Most recent WAL LSN the server has written for each cluster and tier")),

		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("WAL files written (rate)", units.OpsPerSecond,
			query(fmt.Sprintf("sum by (cluster_name, tier) (%s)",
				clusterScope.over("rate", "klio_server_wal_written_total", "$__rate_interval")), "{{cluster_name}} {{tier}}"),
		).Description("Rate of WAL files written by the server, split by cluster and storage tier.")),

		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("WAL files written (total)", units.Number,
			query(fmt.Sprintf("sum by (cluster_name, tier) (%s)",
				clusterScope.series("klio_server_wal_written_total")), "{{cluster_name}} {{tier}}"),
		).Description("Total WAL files written by the server since it last restarted, split by cluster and "+
			"storage tier.")),

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL tier-2 upload duration percentiles (rate)", units.Nanoseconds,
				quantileTargetsWindowIn("klio_server_wal_upload_duration_nanoseconds_bucket", "le, cluster_name",
					clusterScope, "{{cluster_name}}", "rate", "$__rate_interval")...,
			).Description("Percentile duration of the tier-2 archival upload to remote storage, per cluster. "+
				"Reflects recent activity, over the rate interval window.")),

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL tier-2 upload duration percentiles (total)", units.Nanoseconds,
				quantileTargetsAbsoluteIn("klio_server_wal_upload_duration_nanoseconds_bucket", "le, cluster_name",
					clusterScope, "{{cluster_name}}")...,
			).Description("Percentile duration of the tier-2 archival upload to remote storage, per cluster. "+
				"Reflects all activity since the server last restarted.")),
	}
}

// backupMaintenancePanels returns the relay, verification and maintenance
// panels.
func backupMaintenancePanels() []sizedPanel {
	return []sizedPanel{
		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Tier-2 backup relay attempts", units.Number,
			query(
				fmt.Sprintf("sum by (cluster_name, outcome) (%s)", servedScope.series("klio_server_backup_relay_total")),
				"{{cluster_name}} / {{outcome}}",
			),
		).Description("Total tier-2 relay attempts since the server last restarted, per cluster and outcome.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Backup verification runs", units.Number,
			query(
				fmt.Sprintf("sum by (cluster_name, outcome, tier) (%s)",
					servedScope.series("klio_server_backup_verifications_total")),
				"{{cluster_name}} {{tier}} / {{outcome}}",
			),
		).Description("Total base backup verification checks since the server last restarted, per cluster, "+
			"broken down by outcome and tier.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Maintenance runs", units.Number,
			query(
				fmt.Sprintf("sum by (cluster_name, tier, outcome) (%s)",
					servedScope.series("klio_server_backup_maintenance_total")),
				"{{cluster_name}} {{tier}}/{{outcome}}",
			),
		).Description("Total post-backup maintenance runs since the server last restarted, per cluster, "+
			"tier and outcome.")),
	}
}

// servedClusterPanels returns the WAL the servers write for the clusters they
// back up.
func servedClusterPanels() []sizedPanel {
	return []sizedPanel{
		sized(largePanelWidth, largePanelHeight, timeSinceLastWALPanel(servedScope)),
		sized(largePanelWidth, largePanelHeight, walBytesRatePanel(servedScope)),
		sized(largePanelWidth, largePanelHeight, walBytesTotalPanel(servedScope)),
	}
}

// serverHealthPanels returns the uptime and the embedded queue panels.
func serverHealthPanels() []sizedPanel {
	return []sizedPanel{
		sized(largePanelWidth, mediumPanelHeight, statPanel("Server uptime", units.DurationInDaysHoursMinutesSeconds,
			query(fmt.Sprintf("max by (k8s_namespace_name, server) (%s)",
				serverLabel(serverScope.series("klio_server_uptime_seconds"))),
				"{{k8s_namespace_name}}/{{server}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Time since the Klio server process started, per server. A sudden drop means that "+
				"server's StatefulSet restarted.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Queue size (messages)", units.Number,
			query(fmt.Sprintf("sum by (k8s_namespace_name, server, stream) (%s)",
				serverLabel(serverScope.series("klio_server_queue_messages"))),
				"{{k8s_namespace_name}}/{{server}} / {{stream}}"),
		).Description("Messages currently held in each NATS JetStream stream of the embedded queue, per "+
			"server.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Queue size (bytes)", units.BytesIEC,
			query(fmt.Sprintf("sum by (k8s_namespace_name, server, stream) (%s)",
				serverLabel(serverScope.series("klio_server_queue_bytes"))),
				"{{k8s_namespace_name}}/{{server}} / {{stream}}"),
		).Description("Bytes currently held in each NATS JetStream stream of the embedded queue, per server.")),
	}
}

// snapshotPanels returns the Kopia base snapshot panels.
func snapshotPanels() []sizedPanel {
	// Three tiles per row: the per-server legend of Retained snapshots does not
	// fit a narrower tile.
	return []sizedPanel{
		sized(largePanelWidth, largePanelHeight, statPanel("Retained snapshots", units.Number,
			// Grouped by server too: two servers reading the same repository, such
			// as a read-only one, would otherwise count its snapshots twice.
			query(fmt.Sprintf("sum by (cluster, tier, k8s_namespace_name, server) (%s)",
				serverLabel(snapshotCluster(fmt.Sprintf("klio_server_backup_snapshots{%s}", snapshotMatcher)))),
				"{{cluster}} {{tier}} ({{k8s_namespace_name}}/{{server}})"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Total kopia snapshots currently retained per cluster and tier, with the Klio server "+
				"that retains them.")),

		sized(largePanelWidth, largePanelHeight, statPanel("Latest snapshot size", units.BytesIEC,
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_size_bytes{%s}", snapshotMatcher))),
				"{{cluster}} {{tier}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Size of the most recent base backup snapshot on Kopia, not accounting for deduplication.")),

		sized(largePanelWidth, largePanelHeight, statPanel("Latest snapshot files", units.Number,
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_files{%s}", snapshotMatcher))),
				"{{cluster}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Number of files in the most recent base backup snapshot on Kopia.")),

		sized(largePanelWidth, largePanelHeight, statPanel("Latest snapshot dirs", units.Number,
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_dirs{%s}", snapshotMatcher))),
				"{{cluster}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Number of directories in the most recent base backup snapshot on Kopia.")),

		sized(largePanelWidth, largePanelHeight, statPanel("Latest snapshot age", units.DurationInDaysHoursMinutesSeconds,
			query(fmt.Sprintf("time() - max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_timestamp_seconds{%s}", snapshotMatcher))),
				"{{cluster}} {{tier}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Age of the most recent base backup snapshot on Kopia, per cluster and tier.")),

		sized(largePanelWidth, largePanelHeight, statPanel("Oldest snapshot age", units.DurationInDaysHoursMinutesSeconds,
			query(fmt.Sprintf("time() - min by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_oldest_snapshot_timestamp_seconds{%s}", snapshotMatcher))),
				"{{cluster}} {{tier}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Age of the oldest retained base backup snapshot on Kopia, per cluster and tier.")),
	}
}

// walProcessingPanels returns the WAL operation latency panels.
func walProcessingPanels() []sizedPanel {
	return []sizedPanel{
		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL block operation duration percentiles (rate)", units.Nanoseconds,
				quantileTargets("klio_server_wal_block_duration_nanoseconds_bucket", "le, path, stage",
					walMatcher, "{{path}}/{{stage}}")...,
			).Description("Percentile per-block WAL processing duration on the server, split by `path` "+
				"(put ingest / get serve) and `stage`, aggregated across the selected clusters. Reflects recent "+
				"activity, over the rate interval window.")),

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL block operation duration percentiles (total)", units.Nanoseconds,
				quantileTargetsAbsolute("klio_server_wal_block_duration_nanoseconds_bucket", "le, path, stage",
					walMatcher, "{{path}}/{{stage}}")...,
			).Description("Percentile per-block WAL processing duration on the server, split by `path` "+
				"(put ingest / get serve) and `stage`, aggregated across the selected clusters. Reflects all "+
				"activity since the server last restarted.")),

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL file get duration percentiles (rate)", units.Nanoseconds,
				quantileTargets("klio_server_wal_get_duration_nanoseconds_bucket", "le, tier",
					walMatcher, "{{tier}}")...,
			).Description("Percentile duration of a complete WAL file gRPC get, split by the tier that "+
				"served it, aggregated across the selected clusters. Reflects recent activity, over the rate interval window.")),

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL file get duration percentiles (total)", units.Nanoseconds,
				quantileTargetsAbsolute("klio_server_wal_get_duration_nanoseconds_bucket", "le, tier",
					walMatcher, "{{tier}}")...,
			).Description("Percentile duration of a complete WAL file gRPC get, split by the tier that "+
				"served it, aggregated across the selected clusters. Reflects all activity since the server "+
				"last restarted.")),
	}
}
