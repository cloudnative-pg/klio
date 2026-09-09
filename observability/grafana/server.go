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
// embedded NATS JetStream queue. Server-level series (uptime, verifications,
// snapshots, queue) carry no cluster_name and are grouped by service_name (the
// server identity, scoped by $server); the per-cluster WAL and PostgreSQL-backup
// series carry cluster_name and are additionally scoped by $cluster.
func serverPanels() []sizedPanel {
	return []sizedPanel{
		// Compact single/low-cardinality values first (stat tiles and bar
		// gauges), then the wider time series. Server-level values are grouped
		// by service_name so two servers (even two with the same pod host name
		// in different namespaces) never collapse into one number.
		sized(4, panelHeight, statPanel("Server uptime", "dtdurations",
			query(fmt.Sprintf("max by (service_name) (klio_server_uptime_seconds{%s})", serverMatcher),
				"{{service_name}}"),
		).Description("Time since the Klio server process started, per server. A sudden drop means that "+
			"server's StatefulSet restarted.")),
		// A stepped time series shows the timeline per cluster/tier over time, so
		// a promotion or failover is visible as the step where the line jumps,
		// not just the value it currently sits at.
		sized(8, panelHeight, timelinePanel("Latest WAL timeline by cluster and tier",
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_wal_latest_written_timeline{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("PostgreSQL timeline of the latest WAL written per cluster and tier, over time. A step "+
			"up marks a promotion or failover.")),
		sized(4, panelHeight, barGaugePanel("Base snapshots by cluster and tier",
			query(fmt.Sprintf("sum by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_snapshots{%s}", serverMatcher))), "{{cluster}} {{tier}}"),
		).Description("Base backup snapshots currently retained per cluster and tier (the cluster is derived "+
			"from the Kopia snapshot source).")),
		sized(4, panelHeight, statPanel("Latest snapshot size", "bytes",
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_size_bytes{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Description("Size on the backend of the most recent base backup snapshot, per cluster and tier.")),
		sized(4, panelHeight, statPanel("Latest snapshot files", "short",
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_files{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Decimals(0).
			Description("Number of files in the most recent base backup snapshot, per cluster and tier.")),
		sized(4, panelHeight, statPanel("Latest snapshot dirs", "short",
			query(fmt.Sprintf("max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_dirs{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Decimals(0).
			Description("Number of directories in the most recent base backup snapshot, per cluster and tier.")),
		sized(4, panelHeight, statPanel("Latest snapshot age", "dtdurations",
			query(fmt.Sprintf("time() - max by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_latest_snapshot_timestamp_seconds{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Description("Age of the most recent base backup snapshot, per cluster and tier. Should stay below "+
			"the backup interval; a tier-2 value drifting above tier-1 means remote relay is lagging.")),
		sized(4, panelHeight, statPanel("Oldest snapshot age", "dtdurations",
			query(fmt.Sprintf("time() - min by (cluster, tier) (%s)",
				snapshotCluster(fmt.Sprintf("klio_server_backup_oldest_snapshot_timestamp_seconds{%s}", serverMatcher))),
				"{{cluster}} {{tier}}"),
		).Description("Age of the oldest retained base backup snapshot, per cluster and tier, reflecting each "+
			"tier's effective retention horizon.")),

		// Retention window of the physical PostgreSQL backups (distinct from
		// the Kopia base-snapshot gauges above): the klio.server.backup.backups
		// / latest_backup_* / oldest_backup_* family, which carry cluster_name
		// and are scoped by cluster_name via walMatcher.
		sized(4, panelHeight, statPanel("Latest backup age (start)", "dtdurations",
			query(fmt.Sprintf("time() - max by (cluster_name, tier) "+
				"(klio_server_backup_latest_backup_start_time_seconds{%s})", walMatcher), "{{cluster_name}} {{tier}}"),
		).Description("Elapsed time since the most recently retained PostgreSQL backup started, per cluster "+
			"and tier.")),
		sized(4, panelHeight, statPanel("Latest backup age (completion)", "dtdurations",
			query(
				fmt.Sprintf("time() - max by (cluster_name, tier) "+
					"(klio_server_backup_latest_backup_completion_time_seconds{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("Elapsed time since the most recently retained PostgreSQL backup completed, per cluster "+
			"and tier.")),
		sized(4, panelHeight, statPanel("Oldest backup age (start)", "dtdurations",
			query(fmt.Sprintf("time() - min by (cluster_name, tier) "+
				"(klio_server_backup_oldest_backup_start_time_seconds{%s})", walMatcher), "{{cluster_name}} {{tier}}"),
		).Description("Elapsed time since the oldest retained PostgreSQL backup started, per cluster and tier, "+
			"reflecting each tier's effective retention horizon.")),
		sized(4, panelHeight, statPanel("Oldest backup age (completion)", "dtdurations",
			query(
				fmt.Sprintf("time() - min by (cluster_name, tier) "+
					"(klio_server_backup_oldest_backup_completion_time_seconds{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("Elapsed time since the oldest retained PostgreSQL backup completed, per cluster and "+
			"tier, reflecting each tier's effective retention horizon.")),
		sized(8, panelHeight, barGaugePanel("PostgreSQL backups retained by cluster and tier",
			query(fmt.Sprintf("sum by (cluster_name, tier) (klio_server_backup_backups{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Decimals(0).
			Description("Number of PostgreSQL backups currently retained per cluster and tier.")),
		sized(8, panelHeight, timeseriesPanel("Latest backup LSN by cluster and tier", "bytes",
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_latest_backup_start_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} {{tier}} start"),
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_latest_backup_end_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} {{tier}} end"),
		).Description("Start and end LSN of the latest retained PostgreSQL backup, per cluster and tier.")),
		sized(8, panelHeight, timeseriesPanel("Oldest backup LSN by cluster and tier", "bytes",
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_oldest_backup_start_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} {{tier}} start"),
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_oldest_backup_end_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} {{tier}} end"),
		).Description("Start and end LSN of the oldest retained PostgreSQL backup, per cluster and tier.")),
		sized(8, panelHeight, timelinePanel("Backup timeline by cluster and tier",
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_latest_backup_timeline{%s})",
				walMatcher), "{{cluster_name}} {{tier}} latest"),
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_backup_oldest_backup_timeline{%s})",
				walMatcher), "{{cluster_name}} {{tier}} oldest"),
		).Description("PostgreSQL timeline of the latest and oldest retained backup, per cluster and tier, "+
			"over time. The latest and oldest lines diverging means the retention window spans a promotion "+
			"or failover.")),

		// WAL ingest, per cluster and tier.
		sized(8, panelHeight, timeseriesPanel("WAL files written rate by cluster and tier", "wps",
			query(fmt.Sprintf("sum by (cluster_name, tier) (rate(klio_server_wal_written_total{%s}[$__rate_interval]))",
				walMatcher), "{{cluster_name}} {{tier}}"),
		).Description("Rate of WAL files written by the server, split by cluster and storage tier.")),
		sized(8, panelHeight, timeseriesPanel("WAL bytes written rate by cluster and tier", "Bps",
			query(
				fmt.Sprintf("sum by (cluster_name, tier) "+
					"(rate(klio_server_wal_written_size_bytes_total{%s}[$__rate_interval]))", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("Rate of WAL bytes written by the server, split by cluster and storage tier.")),
		sized(8, panelHeight, timeseriesPanel("Time since last WAL written by cluster and tier", "dtdurations",
			query(fmt.Sprintf("time() - max by (cluster_name, tier) (klio_server_wal_latest_written_time_seconds{%s})",
				walMatcher), "{{cluster_name}} {{tier}}"),
		).Description("Elapsed time since the server last wrote a WAL file for each cluster and tier. A stale "+
			"tier-1 value means PostgreSQL stopped shipping WALs; a stale tier-2 value means the remote "+
			"backend stopped receiving them.")),
		sized(8, panelHeight, timeseriesPanel("Latest written LSN by cluster and tier", "bytes",
			query(fmt.Sprintf("max by (cluster_name, tier) (klio_server_wal_latest_written_lsn_bytes{%s})", walMatcher),
				"{{cluster_name}} {{tier}}"),
		).Description("Most recent WAL LSN the server has written for each cluster and tier, as a byte "+
			"offset.")),
		sized(8, panelHeight, timeseriesPanel("Backup verification rate by outcome and tier", "ops",
			query(
				fmt.Sprintf("sum by (service_name, outcome, tier) "+
					"(rate(klio_server_backup_verifications_total{%s}[$__rate_interval]))", serverMatcher),
				"{{service_name}} {{tier}} / {{outcome}}",
			),
		).Description("Rate of base backup verification checks, broken down by outcome and tier (the "+
			"verification counter carries no cluster_name, so it is a per-server signal).")),

		// WAL processing latency, from the per-block and per-file duration
		// histograms. These percentile panels aggregate the WAL distribution of
		// the selected clusters; narrow $cluster to isolate one cluster.
		sized(8, panelHeight, timeseriesPanel("WAL block duration (p50/p95/p99) by path and stage", "ns",
			quantileTargets("klio_server_wal_block_duration_nanoseconds_bucket", "le, path, stage",
				walMatcher, "{{path}}/{{stage}}")...,
		).Description("Percentile per-block WAL processing duration on the server, split by `path` "+
			"(put ingest / get serve) and `stage`, aggregated across the selected clusters.")),
		sized(8, panelHeight, timeseriesPanel("WAL file get duration (p50/p95/p99) by tier", "ns",
			quantileTargets("klio_server_wal_get_duration_nanoseconds_bucket", "le, tier",
				walMatcher, "{{tier}}")...,
		).Description("Percentile duration of a complete WAL file gRPC get, split by the tier that "+
			"served it, aggregated across the selected clusters.")),
		sized(8, panelHeight, timeseriesPanel("WAL tier-2 upload duration (p50/p95/p99) by cluster", "ns",
			quantileTargets("klio_server_wal_upload_duration_nanoseconds_bucket", "le, cluster_name",
				walMatcher, "{{cluster_name}}")...,
		).Description("Percentile duration of the tier-2 archival upload to remote storage, per cluster.")),

		// Post-backup processing: tier-2 relay and per-tier maintenance runs.
		sized(12, panelHeight, timeseriesPanel("Tier-2 relay rate by cluster and outcome", "ops",
			query(
				fmt.Sprintf("sum by (cluster_name, outcome) (rate(klio_server_backup_relay_total{%s}"+
					"[$__rate_interval]))", walMatcher),
				"{{cluster_name}} / {{outcome}}",
			),
		).Description("Rate of tier-2 relay attempts after a backup (migration and verification), per cluster "+
			"and outcome.")),
		sized(12, panelHeight, timeseriesPanel("Maintenance run rate by cluster, tier and outcome", "ops",
			query(
				fmt.Sprintf("sum by (cluster_name, tier, outcome) (rate(klio_server_backup_maintenance_total{%s}"+
					"[$__rate_interval]))", walMatcher),
				"{{cluster_name}} {{tier}}/{{outcome}}",
			),
		).Description("Rate of post-backup maintenance runs (base-snapshot retention and WAL cleanup), "+
			"per cluster, tier and outcome.")),

		// Embedded NATS JetStream queue, per server and stream.
		sized(8, panelHeight, timeseriesPanel("Queue messages by stream", "none",
			query(fmt.Sprintf("sum by (service_name, stream) (klio_server_queue_messages{%s})", serverMatcher),
				"{{service_name}} / {{stream}}"),
		).Description("Messages currently held in each NATS JetStream stream of the embedded queue, per "+
			"server.")),
		sized(8, panelHeight, timeseriesPanel("Queue bytes by stream", "bytes",
			query(fmt.Sprintf("sum by (service_name, stream) (klio_server_queue_bytes{%s})", serverMatcher),
				"{{service_name}} / {{stream}}"),
		).Description("Bytes currently held in each NATS JetStream stream of the embedded queue, per server.")),
	}
}
