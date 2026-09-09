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

// clientPanels returns the "Client / Plugin" section panels. These metrics are
// emitted by the Klio plugin sidecar that runs in each PostgreSQL pod: the
// backup lifecycle (`klio.plugin.backup.*`, exported to Prometheus as
// `klio_plugin_backup_*`) and the WAL streaming client it supervises as a
// child process (`klio.client.wal.*`, exported as `klio_client_wal_*`). Both
// families carry a cluster_name, so every panel groups by cluster_name and is
// scoped by $namespace and $cluster; nothing folds several clusters that share
// a namespace into a single value.
func clientPanels() []sizedPanel {
	return []sizedPanel{
		// Current backup state, grouped by cluster so a namespace hosting
		// several clusters shows one series each instead of a folded total.
		sized(4, panelHeight, statPanel("Backups in progress", "none",
			query(fmt.Sprintf("sum by (cluster_name) (klio_plugin_backup_in_progress{%s})", clientMatcher),
				"{{cluster_name}}"),
		).Decimals(0).
			Description("Base backups currently running, per cluster.")),
		sized(4, panelHeight, statPanel("Time since last successful backup", "dtdurations",
			query(fmt.Sprintf("time() - max by (cluster_name) (klio_plugin_backup_latest_completion_time_seconds{%s})",
				clientMatcher), "{{cluster_name}}"),
		).Description("Elapsed time since the most recent base backup completed successfully, per cluster. A "+
			"value well above the backup interval means that cluster's backups have stopped succeeding.")),
		sized(4, panelHeight, statPanel("Time since last failed backup", "dtdurations",
			query(fmt.Sprintf("time() - max by (cluster_name) (klio_plugin_backup_latest_failure_time_seconds{%s})",
				clientMatcher), "{{cluster_name}}"),
		).Description("Elapsed time since the most recent base backup failure, per cluster. A small value means "+
			"a failure happened recently.")),
		sized(4, panelHeight, statPanel("Latest backup duration", "dtdurations",
			query(fmt.Sprintf("max by (cluster_name) (klio_plugin_backup_latest_duration_seconds{%s})", clientMatcher),
				"{{cluster_name}}"),
		).Description("Wall-clock duration of the most recent base backup, per cluster.")),
		sized(4, panelHeight, statPanel("Time since last backup started", "dtdurations",
			query(fmt.Sprintf("time() - max by (cluster_name) (klio_plugin_backup_latest_start_time_seconds{%s})",
				clientMatcher), "{{cluster_name}}"),
		).Description("Elapsed time since the most recent base backup started, per cluster. Compare against the "+
			"latest duration to tell whether a backup is still running or overdue.")),
		// Derived: share of backup runs that succeeded over the selected range,
		// per cluster.
		sized(4, panelHeight, statPanel("Backup success ratio", "percentunit",
			query(
				fmt.Sprintf("sum by (cluster_name) (increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}"+
					"[$__range])) / clamp_min(sum by (cluster_name) (increase(klio_plugin_backup_runs_total{%s}"+
					"[$__range])), 1)", clientMatcher, clientMatcher),
				"{{cluster_name}}",
			),
		).Description("Fraction of base backup runs that succeeded over the selected time range "+
			"(successful runs / total runs), per cluster.")),
		// Derived: count of successful backups over fixed trailing windows. The
		// runs counter resets when the plugin sidecar restarts, so increase()
		// over long windows is an approximation across restarts.
		sized(4, panelHeight, statPanel("Successful backups (24h)", "none",
			query(fmt.Sprintf("sum by (cluster_name) (increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}"+
				"[24h]))", clientMatcher), "{{cluster_name}}"),
		).Decimals(0).
			Description("Base backups that completed successfully in the last 24 hours, per cluster. Counter "+
				"resets on plugin restart make long-window counts approximate.")),
		sized(4, panelHeight, statPanel("Successful backups (7d)", "none",
			query(fmt.Sprintf("sum by (cluster_name) (increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}"+
				"[7d]))", clientMatcher), "{{cluster_name}}"),
		).Decimals(0).
			Description("Base backups that completed successfully in the last 7 days, per cluster. Counter "+
				"resets on plugin restart make long-window counts approximate.")),

		// Backup throughput and outcomes, split by cluster and outcome/category.
		sized(8, panelHeight, timeseriesPanel("Backup run rate by cluster and outcome", "ops",
			query(fmt.Sprintf("sum by (cluster_name, outcome) (rate(klio_plugin_backup_runs_total{%s}"+
				"[$__rate_interval]))", clientMatcher), "{{cluster_name}} / {{outcome}}"),
		).Description("Rate of base backup runs, broken down by cluster and outcome (success or failure).")),
		sized(8, panelHeight, timeseriesPanel("Backup failure rate by cluster and category", "ops",
			query(
				fmt.Sprintf("sum by (cluster_name, failure_category) "+
					"(rate(klio_plugin_backup_runs_total{outcome=\"failure\",%s}[$__rate_interval]))", clientMatcher),
				"{{cluster_name}} / {{failure_category}}",
			),
		).Description("Rate of failed base backup runs, broken down by cluster and failure category, to show "+
			"why backups are failing.")),
		// Distribution of backup wall-clock durations per cluster. The
		// latest-duration stat tile above shows only the most recent run; these
		// percentiles show the spread and let an admin see backup runtime
		// trending up over time.
		sized(8, panelHeight, timeseriesPanel("Backup duration p95 by cluster", "s",
			query(
				fmt.Sprintf("histogram_quantile(0.95, sum by (le, cluster_name) "+
					"(increase(klio_plugin_backup_duration_seconds_bucket{%s}[$__range])))", clientMatcher),
				"p95 {{cluster_name}}",
			),
		).Description("95th-percentile wall-clock duration of base backup runs (across all outcomes), per "+
			"cluster, computed over the whole selected range so the lines stay populated between runs. Widen "+
			"the dashboard range to span several backups for a stable reading; if the range contains no backup "+
			"for a cluster, that cluster's line is empty.")),
		// Backup volume over time: count of runs per bucket, split by cluster
		// and outcome. Bars aggregate how many backups happened, which is more
		// useful for infrequent backups than instantaneous duration percentiles.
		sized(8, panelHeight, barPanel("Backups by cluster and outcome", "short",
			query(
				fmt.Sprintf("sum by (cluster_name, outcome) (increase(klio_plugin_backup_runs_total{%s}"+
					"[$__interval]))", clientMatcher), "{{cluster_name}} / {{outcome}}"),
		).Description("Count of base backup runs per time bucket, split by cluster and outcome. Bars aggregate "+
			"the number of backups over each interval (per day on a multi-day range).")),

		// WAL streaming client, run as a child process of this same sidecar. A
		// stepped time series shows when the streamed timeline changed (a
		// failover), not just its current value.
		sized(8, panelHeight, timelinePanel("Streaming timeline by cluster",
			query(fmt.Sprintf("max by (cluster_name) (klio_client_wal_timeline{%s})", clientMatcher),
				"{{cluster_name}}"),
		).Description("PostgreSQL timeline the WAL streaming client is streaming, per cluster, over time. A "+
			"step up marks a failover on that cluster.")),
		sized(16, panelHeight, timeseriesPanel("WAL block send duration (p50/p95/p99) by cluster", "ns",
			quantileTargets("klio_client_wal_block_duration_nanoseconds_bucket", "le, cluster_name",
				clientMatcher, "{{cluster_name}}")...,
		).Description("Percentile latency of the client's gRPC send of a WAL block to the server, per "+
			"cluster. Most meaningful under active write load; on an idle or low-write cluster, WAL "+
			"blocks are sent too infrequently for the underlying histogram_quantile to be reliable, so "+
			"the line may look sparse or noisy rather than absent.")),
	}
}
