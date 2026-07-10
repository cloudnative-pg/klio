package main

import (
	"fmt"
)

// serverPanels returns the "Server" section panels. These metrics are emitted
// by the Klio server StatefulSet (the `klio.server.*` family, exported to
// Prometheus as `klio_server_*`): WAL ingest, backup verification, base
// snapshots and the embedded NATS JetStream queue. Queries are scoped by
// $namespace and $server; the WAL series additionally carry cluster_name and
// are scoped by $cluster.
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
		sized(4, panelHeight, barGaugePanel("Latest WAL timeline by tier", "none",
			query(fmt.Sprintf("max by (tier) (klio_server_wal_latest_written_timeline{%s})", walMatcher), "{{tier}}"),
		).Description("Current PostgreSQL timeline of the latest WAL written per tier. A change reflects "+
			"a promotion or failover.")),
		sized(4, panelHeight, barGaugePanel("Base snapshots by tier", "none",
			query(fmt.Sprintf("sum by (tier) (klio_server_backup_snapshots{%s})", serverMatcher), "{{tier}}"),
		).Description("Base backup snapshots currently retained per tier.")),
		sized(4, panelHeight, statPanel("Latest snapshot size", "bytes",
			query(fmt.Sprintf("max(klio_server_backup_latest_snapshot_size_bytes{%s})", serverMatcher), "latest size"),
		).Description("Size on the backend of the most recent base backup snapshot.")),
		sized(4, panelHeight, statPanel("Latest snapshot age", "dtdurations",
			query(fmt.Sprintf("time() - max(klio_server_backup_latest_snapshot_timestamp_seconds{%s})", serverMatcher),
				"latest age"),
		).Description("Age of the most recent base backup snapshot. Should stay below the backup interval.")),
		sized(4, panelHeight, statPanel("Oldest snapshot age", "dtdurations",
			query(fmt.Sprintf("time() - min(klio_server_backup_oldest_snapshot_timestamp_seconds{%s})", serverMatcher),
				"oldest age"),
		).Description("Age of the oldest retained base backup snapshot, reflecting the effective "+
			"retention horizon.")),

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

		// Embedded NATS JetStream queue, broken down by stream.
		sized(8, panelHeight, timeseriesPanel("Queue messages by stream", "none",
			query(fmt.Sprintf("sum by (stream) (klio_server_queue_messages{%s})", serverMatcher), "{{stream}}"),
		).Description("Messages currently held in each NATS JetStream stream of the embedded queue.")),
		sized(8, panelHeight, timeseriesPanel("Queue bytes by stream", "bytes",
			query(fmt.Sprintf("sum by (stream) (klio_server_queue_bytes{%s})", serverMatcher), "{{stream}}"),
		).Description("Bytes currently held in each NATS JetStream stream of the embedded queue.")),
	}
}
