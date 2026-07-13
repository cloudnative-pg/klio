package main

import (
	"fmt"
)

// serverPanels returns the "Server" section panels. These metrics are emitted
// by the Klio server StatefulSet (the `klio.server.*` family, exported to
// Prometheus as `klio_server_*`): WAL ingest, backup verification, base
// snapshots, the retention window of physical PostgreSQL backups, and the
// embedded NATS JetStream queue. Queries are scoped by $namespace and
// $server; the WAL and PostgreSQL-backup series additionally carry
// cluster_name and are scoped by $cluster.
func serverPanels() []sizedPanel {
	return []sizedPanel{
		// Compact single/low-cardinality values first (stat tiles and bar
		// gauges), then the wider time series.
		sized(4, panelHeight, statPanel("Server uptime", "dtdurations",
			query(fmt.Sprintf("max(klio_server_uptime_seconds{%s})", serverMatcher), "uptime"),
		).Description("Time since the Klio server process started. A sudden drop means the server "+
			"StatefulSet restarted.")),
		// A bar gauge shows one labeled bar per tier, so both tiers are always
		// visible (a narrow stat tile hides all but the first series).
		sized(4, panelHeight, barGaugePanel("Latest WAL timeline by tier",
			query(fmt.Sprintf("max by (tier) (klio_server_wal_latest_written_timeline{%s})", walMatcher), "{{tier}}"),
		).Description("Current PostgreSQL timeline of the latest WAL written per tier. A change reflects "+
			"a promotion or failover.")),
		sized(4, panelHeight, barGaugePanel("Base snapshots by tier",
			query(fmt.Sprintf("sum by (tier) (klio_server_backup_snapshots{%s})", serverMatcher), "{{tier}}"),
		).Description("Base backup snapshots currently retained per tier.")),
		sized(4, panelHeight, statPanel("Latest snapshot size", "bytes",
			query(fmt.Sprintf("max(klio_server_backup_latest_snapshot_size_bytes{%s})", serverMatcher), "latest size"),
		).Description("Size on the backend of the most recent base backup snapshot.")),
		sized(4, panelHeight, statPanel("Latest snapshot files", "short",
			query(fmt.Sprintf("max(klio_server_backup_latest_snapshot_files{%s})", serverMatcher), "latest files"),
		).Decimals(0).
			Description("Number of files in the most recent base backup snapshot.")),
		sized(4, panelHeight, statPanel("Latest snapshot dirs", "short",
			query(fmt.Sprintf("max(klio_server_backup_latest_snapshot_dirs{%s})", serverMatcher), "latest dirs"),
		).Decimals(0).
			Description("Number of directories in the most recent base backup snapshot.")),
		sized(4, panelHeight, statPanel("Latest snapshot age", "dtdurations",
			query(fmt.Sprintf("time() - max(klio_server_backup_latest_snapshot_timestamp_seconds{%s})", serverMatcher),
				"latest age"),
		).Description("Age of the most recent base backup snapshot. Should stay below the backup interval.")),
		sized(4, panelHeight, statPanel("Oldest snapshot age", "dtdurations",
			query(fmt.Sprintf("time() - min(klio_server_backup_oldest_snapshot_timestamp_seconds{%s})", serverMatcher),
				"oldest age"),
		).Description("Age of the oldest retained base backup snapshot, reflecting the effective "+
			"retention horizon.")),

		// Retention window of the physical PostgreSQL backups (distinct from
		// the Kopia base-snapshot gauges above): the klio.server.backup.backups
		// / latest_backup_* / oldest_backup_* family, scoped by cluster_name
		// via walMatcher.
		sized(4, panelHeight, statPanel("Latest backup age (start)", "dtdurations",
			query(fmt.Sprintf("time() - max(klio_server_backup_latest_backup_start_time_seconds{%s})", walMatcher),
				"latest start age"),
		).Description("Elapsed time since the most recently retained PostgreSQL backup started.")),
		sized(4, panelHeight, statPanel("Latest backup age (completion)", "dtdurations",
			query(
				fmt.Sprintf("time() - max(klio_server_backup_latest_backup_completion_time_seconds{%s})", walMatcher),
				"latest completion age"),
		).Description("Elapsed time since the most recently retained PostgreSQL backup completed.")),
		sized(4, panelHeight, statPanel("Oldest backup age (start)", "dtdurations",
			query(fmt.Sprintf("time() - min(klio_server_backup_oldest_backup_start_time_seconds{%s})", walMatcher),
				"oldest start age"),
		).Description("Elapsed time since the oldest retained PostgreSQL backup started, reflecting the "+
			"effective retention horizon.")),
		sized(4, panelHeight, statPanel("Oldest backup age (completion)", "dtdurations",
			query(
				fmt.Sprintf("time() - min(klio_server_backup_oldest_backup_completion_time_seconds{%s})", walMatcher),
				"oldest completion age"),
		).Description("Elapsed time since the oldest retained PostgreSQL backup completed, reflecting the "+
			"effective retention horizon.")),
		sized(8, panelHeight, barGaugePanel("PostgreSQL backups retained by tier",
			query(fmt.Sprintf("sum by (tier) (klio_server_backup_backups{%s})", walMatcher), "{{tier}}"),
		).Decimals(0).
			Description("Number of PostgreSQL backups currently retained per tier, across clusters.")),
		sized(8, panelHeight, timeseriesPanel("Latest backup LSN by cluster", "bytes",
			query(fmt.Sprintf("max by (cluster_name) (klio_server_backup_latest_backup_start_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} start"),
			query(fmt.Sprintf("max by (cluster_name) (klio_server_backup_latest_backup_end_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} end"),
		).Description("Start and end LSN of the latest retained PostgreSQL backup, per cluster.")),
		sized(8, panelHeight, timeseriesPanel("Oldest backup LSN by cluster", "bytes",
			query(fmt.Sprintf("max by (cluster_name) (klio_server_backup_oldest_backup_start_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} start"),
			query(fmt.Sprintf("max by (cluster_name) (klio_server_backup_oldest_backup_end_lsn_bytes{%s})",
				walMatcher), "{{cluster_name}} end"),
		).Description("Start and end LSN of the oldest retained PostgreSQL backup, per cluster.")),
		sized(8, panelHeight, barGaugePanel("Backup timeline by cluster",
			query(fmt.Sprintf("max by (cluster_name) (klio_server_backup_latest_backup_timeline{%s})", walMatcher),
				"{{cluster_name}} latest"),
			query(fmt.Sprintf("max by (cluster_name) (klio_server_backup_oldest_backup_timeline{%s})", walMatcher),
				"{{cluster_name}} oldest"),
		).Description("PostgreSQL timeline of the latest and oldest retained backup, per cluster. A "+
			"mismatch means the retention window spans a promotion or failover.")),

		// WAL ingest, unified across tiers via the `tier` label.
		sized(8, panelHeight, timeseriesPanel("WAL files written rate by tier", "wps",
			query(fmt.Sprintf("sum by (tier) (rate(klio_server_wal_written_total{%s}[$__rate_interval]))", walMatcher),
				"{{tier}}"),
		).Description("Rate of WAL files written by the server, split by storage tier.")),
		sized(8, panelHeight, timeseriesPanel("WAL bytes written rate by tier", "Bps",
			query(
				fmt.Sprintf("sum by (tier) (rate(klio_server_wal_written_size_bytes_total{%s}[$__rate_interval]))",
					walMatcher),
				"{{tier}}"),
		).Description("Rate of WAL bytes written by the server, split by storage tier.")),
		sized(8, panelHeight, timeseriesPanel("Time since last WAL written by tier", "dtdurations",
			query(fmt.Sprintf("time() - max by (tier) (klio_server_wal_latest_written_time_seconds{%s})", walMatcher),
				"{{tier}}"),
		).Description("Elapsed time since the server last wrote a WAL file for each tier. A stale tier-1 "+
			"value means PostgreSQL stopped shipping WALs; a stale tier-2 value means the remote backend "+
			"stopped receiving them.")),
		sized(8, panelHeight, timeseriesPanel("Latest written LSN by tier", "bytes",
			query(fmt.Sprintf("max by (tier) (klio_server_wal_latest_written_lsn_bytes{%s})", walMatcher), "{{tier}}"),
		).Description("Most recent WAL LSN the server has written for each tier, as a byte offset.")),
		sized(8, panelHeight, timeseriesPanel("Backup verification rate by outcome and tier", "ops",
			query(
				fmt.Sprintf("sum by (outcome, tier) (rate(klio_server_backup_verifications_total{%s}[$__rate_interval]))",
					serverMatcher),
				"{{tier}} / {{outcome}}",
			),
		).Description("Rate of base backup verification checks, broken down by outcome and tier.")),

		// WAL processing latency, from the per-block and per-file duration
		// histograms introduced alongside the unified WAL ingest series.
		sized(8, panelHeight, timeseriesPanel("WAL block duration (p50/p95/p99) by path and stage", "ns",
			quantileTargets("klio_server_wal_block_duration_nanoseconds_bucket", "le, path, stage",
				walMatcher, "{{path}}/{{stage}}")...,
		).Description("Percentile per-block WAL processing duration on the server, split by `path` "+
			"(put ingest / get serve) and `stage`.")),
		sized(8, panelHeight, timeseriesPanel("WAL file get duration (p50/p95/p99) by tier", "ns",
			quantileTargets("klio_server_wal_get_duration_nanoseconds_bucket", "le, tier",
				walMatcher, "{{tier}}")...,
		).Description("Percentile duration of a complete WAL file gRPC get, split by the tier that "+
			"served it.")),
		sized(8, panelHeight, timeseriesPanel("WAL tier-2 upload duration (p50/p95/p99) by cluster", "ns",
			quantileTargets("klio_server_wal_upload_duration_nanoseconds_bucket", "le, cluster_name",
				walMatcher, "{{cluster_name}}")...,
		).Description("Percentile duration of the tier-2 archival upload to remote storage, per cluster.")),

		// Post-backup processing: tier-2 relay and per-tier maintenance runs.
		sized(12, panelHeight, timeseriesPanel("Tier-2 relay rate by outcome", "ops",
			query(
				fmt.Sprintf("sum by (outcome) (rate(klio_server_backup_relay_total{%s}[$__rate_interval]))", walMatcher),
				"{{outcome}}",
			),
		).Description("Rate of tier-2 relay attempts after a backup (migration and verification), by outcome.")),
		sized(12, panelHeight, timeseriesPanel("Maintenance run rate by tier and outcome", "ops",
			query(
				fmt.Sprintf("sum by (tier, outcome) (rate(klio_server_backup_maintenance_total{%s}[$__rate_interval]))",
					walMatcher),
				"{{tier}} / {{outcome}}",
			),
		).Description("Rate of post-backup maintenance runs (base-snapshot retention and WAL cleanup), "+
			"by tier and outcome.")),

		// Embedded NATS JetStream queue, broken down by stream.
		sized(8, panelHeight, timeseriesPanel("Queue messages by stream", "none",
			query(fmt.Sprintf("sum by (stream) (klio_server_queue_messages{%s})", serverMatcher), "{{stream}}"),
		).Description("Messages currently held in each NATS JetStream stream of the embedded queue.")),
		sized(8, panelHeight, timeseriesPanel("Queue bytes by stream", "bytes",
			query(fmt.Sprintf("sum by (stream) (klio_server_queue_bytes{%s})", serverMatcher), "{{stream}}"),
		).Description("Bytes currently held in each NATS JetStream stream of the embedded queue.")),
	}
}
