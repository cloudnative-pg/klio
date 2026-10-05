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

// serverPanels returns the "Server" section panels. These metrics are emitted
// by the Klio server StatefulSet (the `klio.server.*` family, exported to
// Prometheus as `klio_server_*`): WAL ingest, backup verification, base
// snapshots, the retention window of physical PostgreSQL backups, and the
// embedded NATS JetStream queue. Server-level series (uptime, snapshots,
// queue) carry no cluster_name and are grouped by service_name (the server
// identity, scoped by $server); the per-cluster WAL, PostgreSQL-backup and
// verification series carry cluster_name and are additionally scoped by
// $cluster.
func serverPanels() []sizedPanel {
	return []sizedPanel{
		sized(gridWidth, descriptionPanelHeight, descriptionPanel(
			"State of the Klio server: WAL ingest, retained backups and snapshots, and the internal queue.")),
		// Overview of the server and of its retention window. Stat tiles are laid out
		// horizontally so that, with several clusters or tiers selected, every series
		// stays visible instead of stacking vertically and clipping below the fixed
		// panel height. Server-level values are grouped by service_name so two servers
		// (even two with the same pod host name in different namespaces) never
		// collapse into one number.
		sized(largePanelWidth, mediumPanelHeight, statPanel("Server uptime", units.DurationInDaysHoursMinutesSeconds,
			query(fmt.Sprintf("max by (service_name) (klio_server_uptime_seconds{%s})", serverMatcher),
				"{{service_name}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Time since the Klio server process started, per server. A sudden drop means that "+
				"server's StatefulSet restarted.")),

		sized(largePanelWidth, mediumPanelHeight, statPanel("Retained backups", units.Number,
			query(fmt.Sprintf("sum by (cluster_name, tier) (klio_server_backup_backups{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Number of PostgreSQL backups currently retained per cluster and tier.")),

		sized(largePanelWidth, mediumPanelHeight,
			statPanel("Oldest restorable point", units.DatetimeISO,
				query(
					fmt.Sprintf("min by (cluster_name, tier) "+
						"(klio_server_backup_oldest_backup_completion_time_seconds{%s}) * 1000", walMatcher),
					"{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Timestamp at which the oldest retained PostgreSQL backup completed, per cluster and "+
					"tier. It is the earliest point in time a restore can target, as long as all the WAL files "+
					"since the backup started are still retained.")),

		// Latest retained backup.
		sized(largePanelWidth, mediumPanelHeight,
			statPanel("Latest backup age (completion)", units.DurationInDaysHoursMinutesSeconds,
				query(
					fmt.Sprintf("time() - max by (cluster_name, tier) "+
						"(klio_server_backup_latest_backup_completion_time_seconds{%s})", walMatcher),
					"{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the most recently retained PostgreSQL backup completed, per cluster "+
					"and tier.")),

		sized(largePanelWidth, mediumPanelHeight,
			statPanel("Latest backup age (start)", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("time() - max by (cluster_name, tier) "+
					"(klio_server_backup_latest_backup_start_time_seconds{%s})", walMatcher), "{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the most recently retained PostgreSQL backup started, per cluster "+
					"and tier.")),

		sized(largePanelWidth, mediumPanelHeight, statPanel("Latest backup timeline", units.Number,
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_latest_backup_timeline{%s})",
				walMatcher), "{{cluster_name}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("PostgreSQL timeline of the latest retained backup, per cluster and tier. Differing "+
				"from the oldest timeline means the retention window spans a promotion or failover.")),

		sized(gridWidth, mediumPanelHeight, lsnTablePanel("Latest backup LSN",
			lsnColumn{"start", fmt.Sprintf(
				"max by (cluster_name, tier) (klio_server_backup_latest_backup_start_lsn_bytes{%s})", walMatcher)},
			lsnColumn{"end", fmt.Sprintf(
				"max by (cluster_name, tier) (klio_server_backup_latest_backup_end_lsn_bytes{%s})", walMatcher)},
		).Description("Start and end LSN of the latest retained PostgreSQL backup, per cluster and tier.")),

		// Oldest retained backup, i.e. the edge of the retention window.
		sized(largePanelWidth, mediumPanelHeight,
			statPanel("Oldest backup age (completion)", units.DurationInDaysHoursMinutesSeconds,
				query(
					fmt.Sprintf("time() - min by (cluster_name, tier) "+
						"(klio_server_backup_oldest_backup_completion_time_seconds{%s})", walMatcher),
					"{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the oldest retained PostgreSQL backup completed, per cluster and "+
					"tier, reflecting each tier's effective retention horizon.")),

		sized(largePanelWidth, mediumPanelHeight,
			statPanel("Oldest backup age (start)", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("time() - min by (cluster_name, tier) "+
					"(klio_server_backup_oldest_backup_start_time_seconds{%s})", walMatcher), "{{cluster_name}} {{tier}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the oldest retained PostgreSQL backup started, per cluster and tier, "+
					"reflecting each tier's effective retention horizon.")),

		sized(largePanelWidth, mediumPanelHeight, statPanel("Oldest backup timeline", units.Number,
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_oldest_backup_timeline{%s})",
				walMatcher), "{{cluster_name}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("PostgreSQL timeline of the oldest retained backup, per cluster and tier. Differing "+
				"from the latest timeline means the retention window spans a promotion or failover.")),

		sized(gridWidth, mediumPanelHeight, lsnTablePanel("Oldest backup LSN",
			lsnColumn{"start", fmt.Sprintf(
				"max by (cluster_name, tier) (klio_server_backup_oldest_backup_start_lsn_bytes{%s})", walMatcher)},
			lsnColumn{"end", fmt.Sprintf(
				"max by (cluster_name, tier) (klio_server_backup_oldest_backup_end_lsn_bytes{%s})", walMatcher)},
		).Description("Start and end LSN of the oldest retained PostgreSQL backup, per cluster and tier.")),

		// Base snapshots on Kopia.
		sized(smallPanelWidth, mediumPanelHeight, statPanel("Retained snapshots", units.Number,
			query(fmt.Sprintf("sum by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_snapshots{%s}", serverMatcher))), "{{cluster}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Total kopia snapshots currently retained per cluster and tier.")),

		sized(smallPanelWidth, mediumPanelHeight, statPanel("Latest snapshot size", units.BytesIEC,
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_size_bytes{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Size of the most recent base backup snapshot on Kopia, not accounting for deduplication.")),

		sized(smallPanelWidth, mediumPanelHeight, statPanel("Latest snapshot files", units.Number,
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_files{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Number of files in the most recent base backup snapshot on Kopia.")),

		sized(smallPanelWidth, mediumPanelHeight, statPanel("Latest snapshot dirs", units.Number,
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_dirs{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Number of directories in the most recent base backup snapshot on Kopia.")),

		sized(smallPanelWidth, mediumPanelHeight, statPanel("Latest snapshot age", units.DurationInDaysHoursMinutesSeconds,
			query(fmt.Sprintf("time() - max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_timestamp_seconds{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Age of the most recent base backup snapshot on Kopia, per cluster and tier.")),

		sized(smallPanelWidth, mediumPanelHeight, statPanel("Oldest snapshot age", units.DurationInDaysHoursMinutesSeconds,
			query(fmt.Sprintf("time() - min by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_oldest_snapshot_timestamp_seconds{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Orientation(common.VizOrientationHorizontal).
			Description("Age of the oldest retained base backup snapshot on Kopia, per cluster and tier.")),

		// WAL ingest.
		sized(largePanelWidth, mediumPanelHeight, timelinePanel("WAL timeline",
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_wal_latest_written_timeline{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("PostgreSQL timeline of the latest WAL written per cluster and tier")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Time since last WAL written", units.DurationSeconds,
			query(fmt.Sprintf("time() - max by (cluster_name, tier) (klio_server_wal_latest_written_time_seconds{%s})",
				walMatcher), "{{cluster_name}} {{tier}}"),
		).Description("Elapsed time since the server last wrote a WAL file for each cluster and tier.")),

		sized(largePanelWidth, mediumPanelHeight, lsnTablePanel("Latest written LSN",
			lsnColumn{"written", fmt.Sprintf(
				"max by (cluster_name, tier) (klio_server_wal_latest_written_lsn_bytes{%s})", walMatcher)},
		).Description("Most recent WAL LSN the server has written for each cluster and tier")),

		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("WAL files written (rate)", units.OpsPerSecond,
			query(fmt.Sprintf("sum by (cluster_name, tier) (rate(klio_server_wal_written_total{%s}[$__rate_interval]))",
				walMatcher), "{{cluster_name}} {{tier}}"),
		).Description("Rate of WAL files written by the server, split by cluster and storage tier.")),

		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("WAL files written (total)", units.Number,
			query(fmt.Sprintf("sum by (cluster_name, tier) (klio_server_wal_written_total{%s})",
				walMatcher), "{{cluster_name}} {{tier}}"),
		).Description("Total WAL files written by the server since it last restarted, split by cluster and "+
			"storage tier.")),

		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("WAL bytes written (rate)", units.BytesPerSecondIEC,
			query(
				fmt.Sprintf("sum by (cluster_name, tier) "+
					"(rate(klio_server_wal_written_size_bytes_total{%s}[$__rate_interval]))", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("Rate of WAL bytes written by the server, split by cluster and storage tier.")),

		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("WAL bytes written (total)", units.BytesIEC,
			query(
				fmt.Sprintf("sum by (cluster_name, tier) (klio_server_wal_written_size_bytes_total{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("Total WAL bytes written by the server since it last restarted, split by cluster and "+
			"storage tier.")),

		// WAL processing duration, each (rate) panel next to its (total) counterpart.
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

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL tier-2 upload duration percentiles (rate)", units.Nanoseconds,
				quantileTargets("klio_server_wal_upload_duration_nanoseconds_bucket", "le, cluster_name",
					walMatcher, "{{cluster_name}}")...,
			).Description("Percentile duration of the tier-2 archival upload to remote storage, per cluster. "+
				"Reflects recent activity, over the rate interval window.")),

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL tier-2 upload duration percentiles (total)", units.Nanoseconds,
				quantileTargetsAbsolute("klio_server_wal_upload_duration_nanoseconds_bucket", "le, cluster_name",
					walMatcher, "{{cluster_name}}")...,
			).Description("Percentile duration of the tier-2 archival upload to remote storage, per cluster. "+
				"Reflects all activity since the server last restarted.")),

		// Async operations.
		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Tier-2 backup relay attempts", units.Number,
			query(
				fmt.Sprintf("sum by (cluster_name, outcome) (klio_server_backup_relay_total{%s})", walMatcher),
				"{{cluster_name}} / {{outcome}}",
			),
		).Description("Total tier-2 relay attempts since the server last restarted, per cluster and outcome.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Backup verification runs", units.Number,
			query(
				fmt.Sprintf("sum by (cluster_name, outcome, tier) (klio_server_backup_verifications_total{%s})", walMatcher),
				"{{cluster_name}} {{tier}} / {{outcome}}",
			),
		).Description("Total base backup verification checks since the server last restarted, per cluster, "+
			"broken down by outcome and tier.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Maintenance runs", units.Number,
			query(
				fmt.Sprintf("sum by (cluster_name, tier, outcome) (klio_server_backup_maintenance_total{%s})", walMatcher),
				"{{cluster_name}} {{tier}}/{{outcome}}",
			),
		).Description("Total post-backup maintenance runs since the server last restarted, per cluster, "+
			"tier and outcome.")),

		// Embedded NATS JetStream queue, per server and stream.
		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("Queue size (messages)", units.Number,
			query(fmt.Sprintf("sum by (service_name, stream) (klio_server_queue_messages{%s})", serverMatcher),
				"{{service_name}} / {{stream}}"),
		).Description("Messages currently held in each NATS JetStream stream of the embedded queue, per "+
			"server.")),

		sized(largestPanelWidth, mediumPanelHeight, timeseriesPanel("Queue size (bytes)", units.BytesIEC,
			query(fmt.Sprintf("sum by (service_name, stream) (klio_server_queue_bytes{%s})", serverMatcher),
				"{{service_name}} / {{stream}}"),
		).Description("Bytes currently held in each NATS JetStream stream of the embedded queue, per server.")),
	}
}
