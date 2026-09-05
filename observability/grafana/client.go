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

// clientWalMatcher selects the WAL streaming client's per-cluster series
// (klio_client_wal_*). The streaming client runs as a child process of the
// plugin sidecar (spawned via `klio send-wal`, same container, different
// PID), but unlike klio_plugin_backup_* it carries cluster_name and no
// host_name, so these panels are scoped by $namespace and $cluster.
const clientWalMatcher = `k8s_namespace_name=~"$namespace",cluster_name=~"$cluster"`

// clientPanels returns the "Client / Plugin" section panels. These metrics are
// emitted by the Klio plugin sidecar that runs in each PostgreSQL pod: the
// backup lifecycle (`klio.plugin.backup.*`, exported to Prometheus as
// `klio_plugin_backup_*`) and the WAL streaming client it supervises as a
// child process (`klio.client.wal.*`, exported as `klio_client_wal_*`).
// It also records the end-to-end WAL restore latency the plugin serves to
// PostgreSQL (`klio.plugin.wal.*`, exported as `klio_plugin_wal_*`).
// Backup and plugin WAL queries are scoped by $namespace; WAL streaming
// queries additionally carry cluster_name and are scoped by $cluster.
func clientPanels() []sizedPanel {
	return []sizedPanel{
		// Current backup state.
		sized(4, panelHeight, statPanel("Backups in progress", "none",
			query(fmt.Sprintf("sum(klio_plugin_backup_in_progress{%s})", nsMatcher), "in progress"),
		).Decimals(0).
			Description("Base backups currently running across the plugin sidecars in the namespace.")),
		sized(4, panelHeight, statPanel("Time since last successful backup", "dtdurations",
			query(fmt.Sprintf("time() - max(klio_plugin_backup_latest_completion_time_seconds{%s})", nsMatcher),
				"since success"),
		).Description("Elapsed time since the most recent base backup completed successfully. A value well "+
			"above the backup interval means backups have stopped succeeding.")),
		sized(4, panelHeight, statPanel("Time since last failed backup", "dtdurations",
			query(fmt.Sprintf("time() - max(klio_plugin_backup_latest_failure_time_seconds{%s})", nsMatcher),
				"since failure"),
		).Description("Elapsed time since the most recent base backup failure. A small value means a "+
			"failure happened recently.")),
		sized(4, panelHeight, statPanel("Latest backup duration", "dtdurations",
			query(fmt.Sprintf("max(klio_plugin_backup_latest_duration_seconds{%s})", nsMatcher), "latest duration"),
		).Description("Wall-clock duration of the most recent base backup.")),
		sized(4, panelHeight, statPanel("Time since last backup started", "dtdurations",
			query(fmt.Sprintf("time() - max(klio_plugin_backup_latest_start_time_seconds{%s})", nsMatcher),
				"since start"),
		).Description("Elapsed time since the most recent base backup started. Compare against the latest "+
			"duration to tell whether a backup is still running or overdue.")),
		// Derived: share of backup runs that succeeded over the selected range.
		sized(4, panelHeight, statPanel("Backup success ratio", "percentunit",
			query(
				fmt.Sprintf("sum(increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}[$__range])) / "+
					"clamp_min(sum(increase(klio_plugin_backup_runs_total{%s}[$__range])), 1)", nsMatcher, nsMatcher),
				"success ratio",
			),
		).Description("Fraction of base backup runs that succeeded over the selected time range "+
			"(successful runs / total runs).")),
		// Derived: count of successful backups over fixed trailing windows. The
		// runs counter resets when the plugin sidecar restarts, so increase()
		// over long windows is an approximation across restarts.
		sized(4, panelHeight, statPanel("Successful backups (24h)", "none",
			query(fmt.Sprintf("sum(increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}[24h]))", nsMatcher),
				"last 24h"),
		).Decimals(0).
			Description("Base backups that completed successfully in the last 24 hours. Counter resets on "+
				"plugin restart make long-window counts approximate.")),
		sized(4, panelHeight, statPanel("Successful backups (7d)", "none",
			query(fmt.Sprintf("sum(increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}[7d]))", nsMatcher),
				"last 7d"),
		).Decimals(0).
			Description("Base backups that completed successfully in the last 7 days. Counter resets on "+
				"plugin restart make long-window counts approximate.")),

		// Backup throughput and outcomes.
		sized(8, panelHeight, timeseriesPanel("Backup run rate by outcome", "ops",
			query(fmt.Sprintf("sum by (outcome) (rate(klio_plugin_backup_runs_total{%s}[$__rate_interval]))", nsMatcher),
				"{{outcome}}"),
		).Description("Rate of base backup runs, broken down by outcome (success or failure).")),
		sized(8, panelHeight, timeseriesPanel("Backup failure rate by category", "ops",
			query(
				fmt.Sprintf("sum by (failure_category) "+
					"(rate(klio_plugin_backup_runs_total{outcome=\"failure\",%s}[$__rate_interval]))", nsMatcher),
				"{{failure_category}}",
			),
		).Description("Rate of failed base backup runs, broken down by failure category, to show why "+
			"backups are failing.")),
		// Distribution of backup wall-clock durations. The latest-duration stat
		// tile above shows only the most recent run; these percentiles show the
		// spread and let an admin see backup runtime trending up over time.
		sized(8, panelHeight, timeseriesPanel("Backup duration (p50/p95/p99)", "s",
			quantileTargetsRange("klio_plugin_backup_duration_seconds_bucket", "le", nsMatcher, "")...,
		).Description("Percentile wall-clock duration of base backup runs (across all outcomes), computed "+
			"over the whole selected range so the lines stay populated between runs. Widen the dashboard "+
			"range to span several backups for a stable reading; if the range contains no backup, the "+
			"panel is empty.")),
		// Backup volume over time: count of runs per bucket, split by outcome.
		// Bars aggregate how many backups happened, which is more useful for
		// infrequent backups than instantaneous duration percentiles.
		sized(8, panelHeight, barPanel("Backups by outcome", "short",
			query(
				fmt.Sprintf("sum by (outcome) (increase(klio_plugin_backup_runs_total{%s}[$__interval]))", nsMatcher),
				"{{outcome}}"),
		).Description("Count of base backup runs per time bucket, split by outcome. Bars aggregate the "+
			"number of backups over each interval (per day on a multi-day range).")),

		// WAL streaming client, run as a child process of this same sidecar.
		sized(8, panelHeight, barGaugePanel("Streaming timeline by cluster",
			query(fmt.Sprintf("max by (cluster_name) (klio_client_wal_timeline{%s})", clientWalMatcher),
				"{{cluster_name}}"),
		).Description("PostgreSQL timeline the WAL streaming client is currently streaming, per cluster.")),
		sized(16, panelHeight, timeseriesPanel("WAL block send duration (p50/p95/p99) by cluster", "ns",
			quantileTargets("klio_client_wal_block_duration_nanoseconds_bucket", "le, cluster_name",
				clientWalMatcher, "{{cluster_name}}")...,
		).Description("Percentile latency of the client's gRPC send of a WAL block to the server, per "+
			"cluster. Most meaningful under active write load; on an idle or low-write cluster, WAL "+
			"blocks are sent too infrequently for the underlying histogram_quantile to be reliable, so "+
			"the line may look sparse or noisy rather than absent.")),

		// WAL restore latency and throughput. The plugin records
		// klio.plugin.wal.restore_duration for every RESTORE_WAL request it serves
		// to PostgreSQL, exported as the klio_plugin_wal_restore_duration_nanoseconds
		// histogram. Scoped by $namespace like the backup family.
		sized(8, panelHeight, timeseriesPanel("WAL restore latency (p95) by cache hit", "ns",
			query(
				fmt.Sprintf("histogram_quantile(0.95, sum by (le, cache_hit) "+
					"(rate(klio_plugin_wal_restore_duration_nanoseconds_bucket{%s}[$__rate_interval])))", nsMatcher),
				"p95 cache_hit={{cache_hit}}"),
		).Description("95th-percentile end-to-end WAL restore latency measured by the plugin: the latency "+
			"PostgreSQL actually experiences, and in a replica cluster the speed of the replication path "+
			"itself. cache_hit=true means the segment was already in the prefetch spool and the restore "+
			"was a local rename; cache_hit=false means PostgreSQL had to wait on a download, so a falling "+
			"hit ratio means prefetch is not keeping ahead of replay and the prefetch count may need "+
			"raising. The two are orders of magnitude apart, hence the split rather than one pooled line.")),
		sized(8, panelHeight, timeseriesPanel("WAL restore hit ratio", "%",
			query(
				fmt.Sprintf("sum(rate(klio_plugin_wal_restore_duration_nanoseconds_count{%s,cache_hit=\"true\"}[$__rate_interval])) / "+
					"sum(rate(klio_plugin_wal_restore_duration_nanoseconds_count{%s}[$__rate_interval])) * 100", nsMatcher, nsMatcher),
				"hit ratio"),
		).Description("Fraction of WAL restores served directly from the prefetch spool instead of waiting on a download. A falling ratio means prefetch is not keeping pace with PostgreSQL replay.")),
		sized(8, panelHeight, timeseriesPanel("WAL restore rate by outcome", "ops",
			query(
				fmt.Sprintf("sum by (outcome) "+
					"(rate(klio_plugin_wal_restore_duration_nanoseconds_count{%s}[$__rate_interval]))", nsMatcher),
				"{{outcome}}"),
		).Description("Rate of WAL restore requests handled by the plugin, split by outcome (success or "+
			"failure).")),
	}
}
