package main

import (
	"fmt"
)

// cnpgMatcher selects the CloudNativePG replication metrics for Klio's WAL
// streaming client. Unlike the klio_* metrics (which carry the OpenTelemetry
// k8s_namespace_name resource attribute), the cnpg_* metrics come from
// CloudNativePG's own exporter and use the `namespace` label; application_name
// is the replication application name Klio connects with.
const cnpgMatcher = `namespace=~"$namespace",application_name="klio"`

// replicationPanels returns the "WAL Replication Lag" section panels. They
// measure how far Klio's WAL streaming client trails the PostgreSQL primary,
// using CloudNativePG's pg_stat_replication metrics (exported to Prometheus as
// cnpg_pg_stat_replication_*). They require CloudNativePG monitoring to be
// scraped into the same Prometheus as Klio's metrics.
func replicationPanels() []sizedPanel {
	return []sizedPanel{
		// WAL lag as the byte distance between the primary's current LSN and
		// the LSN Klio's streaming client has flushed. Reported in bytes;
		// Grafana scales the unit for display.
		sized(8, panelHeight, timeseriesPanel("Tier-1 Replication lag (Bytes)", "bytes",
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_flush_diff_bytes{%s}", cnpgMatcher),
				"{{pod}}"),
		).Description("Byte distance between the primary's current WAL LSN and the LSN Klio's streaming "+
			"client has flushed, from CloudNativePG's pg_stat_replication.")),
		// Flush lag in seconds: time between a commit on the primary and Klio's
		// client flushing the corresponding WAL.
		sized(8, panelHeight, timeseriesPanel("Tier-1 Replication lag (Seconds)", "s",
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_flush_lag_seconds{%s}", cnpgMatcher),
				"{{pod}}"),
		).Description("Time between a commit on the primary and Klio's streaming client flushing the "+
			"corresponding WAL, from CloudNativePG's pg_stat_replication.")),
		// Derived: tier-2 archival backlog as the LSN gap between what the
		// server has on local disk (tier1) and what has been archived remotely
		// (tier2), in MiB. max by (cluster_name) collapses each tier to one
		// series per cluster so the subtraction is one-to-one even when several
		// namespaces or server instances export the metric.
		sized(8, panelHeight, timeseriesPanel("Tier-2 archival lag (Tier-1 LSN gap)", "mbytes",
			query(
				fmt.Sprintf(
					"(klio_server_wal_latest_written_lsn_bytes{tier=\"tier1\",%s} - "+
						"on (cluster_name) klio_server_wal_latest_written_lsn_bytes{tier=\"tier2\",%s}) "+
						"/ 1024 / 1024",
					walMatcher, walMatcher),
				"{{cluster_name}}",
			),
		).Description("LSN distance between tier 1 (local disk) and tier 2 (remote storage) per cluster. "+
			"A growing gap means remote archival is falling behind.")),
	}
}
