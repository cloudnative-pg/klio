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

// clientPanels returns the "Client / Plugin" section panels. These metrics are
// emitted by the Klio plugin sidecar that runs in each PostgreSQL pod: the
// backup lifecycle (`klio.plugin.backup.*`, exported to Prometheus as
// `klio_plugin_backup_*`) and the WAL streaming client it supervises as a
// child process (`klio.client.wal.*`, exported as `klio_client_wal_*`), and
// the WAL restores it serves to PostgreSQL (`klio.plugin.wal.*`, exported as
// `klio_plugin_wal_*`). All three families carry a cluster_name, so every
// panel groups by cluster_name and is scoped by $namespace and $cluster;
// nothing folds several clusters that share a namespace into a single value.
func clientPanels() []sizedPanel {
	return []sizedPanel{
		sized(gridWidth, descriptionPanelHeight, descriptionPanel(
			"Backup lifecycle, WAL streaming and WAL restore as seen by the plugin sidecar running in each "+
				"PostgreSQL pod.")),
		// Backup activity.
		// Current backup state, grouped by cluster so a namespace hosting
		// several clusters shows one series each instead of a folded total.
		// Orientation forced to horizontal: with several clusters selected,
		// stat panels laid out "auto" stack tiles vertically and clip inside
		// the fixed panel height, hiding clusters below the fold.
		sized(mediumPanelWidth, mediumPanelHeight, statPanel("Backups in progress", units.Number,
			query(fmt.Sprintf("sum by (cluster_name) (klio_plugin_backup_in_progress{%s})", clientMatcher),
				"{{cluster_name}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Base backups currently running, per cluster.")),

		sized(mediumPanelWidth, mediumPanelHeight, statPanel("Successful backup runs", units.Number,
			query(fmt.Sprintf("sum by (cluster_name) (increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}"+
				"[$__range]))", clientMatcher), "{{cluster_name}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Base backups completed successfully by the running clients sidecars, per cluster, "+
				"over the selected time range.")),

		sized(mediumPanelWidth, mediumPanelHeight, statPanel("Failed backup runs", units.Number,
			query(fmt.Sprintf("sum by (cluster_name) (increase(klio_plugin_backup_runs_total{outcome=\"failure\",%s}"+
				"[$__range]))", clientMatcher), "{{cluster_name}}"),
		).Decimals(0).
			Orientation(common.VizOrientationHorizontal).
			Description("Base backups that failed on the running clients sidecars, per cluster, "+
				"over the selected time range.")),

		sized(mediumPanelWidth, mediumPanelHeight, statPanel("Backup run success ratio", units.PercentUnit,
			query(
				fmt.Sprintf("sum by (cluster_name) (increase(klio_plugin_backup_runs_total{outcome=\"success\",%s}"+
					"[$__range])) / clamp_min(sum by (cluster_name) (increase(klio_plugin_backup_runs_total{%s}"+
					"[$__range])), 1)", clientMatcher, clientMatcher),
				"{{cluster_name}}",
			),
		).Orientation(common.VizOrientationHorizontal).
			Description("Fraction of base backup runs that succeeded over the selected time range "+
				"(successful runs / total runs), per cluster.")),

		// State of the latest backup.
		sized(mediumPanelWidth, mediumPanelHeight,
			statPanel("Latest backup duration", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("max by (cluster_name) (klio_plugin_backup_latest_duration_seconds{%s})", clientMatcher),
					"{{cluster_name}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Wall-clock duration of the most recent base backup, per cluster.")),

		sized(mediumPanelWidth, mediumPanelHeight,
			statPanel("Latest successful backup age", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("time() - max by (cluster_name) (klio_plugin_backup_latest_completion_time_seconds{%s})",
					clientMatcher), "{{cluster_name}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the most recent base backup completed successfully, per cluster. A "+
					"value well above the backup interval means that cluster's backups have stopped succeeding.")),

		sized(mediumPanelWidth, mediumPanelHeight,
			statPanel("Latest failed backup age", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("time() - max by (cluster_name) (klio_plugin_backup_latest_failure_time_seconds{%s})",
					clientMatcher), "{{cluster_name}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the most recent base backup failure, per cluster. A small value means "+
					"a failure happened recently.")),

		sized(mediumPanelWidth, mediumPanelHeight,
			statPanel("Latest backup start age", units.DurationInDaysHoursMinutesSeconds,
				query(fmt.Sprintf("time() - max by (cluster_name) (klio_plugin_backup_latest_start_time_seconds{%s})",
					clientMatcher), "{{cluster_name}}"),
			).Orientation(common.VizOrientationHorizontal).
				Description("Elapsed time since the most recent base backup started, per cluster. Compare against the "+
					"latest duration to tell whether a backup is still running or overdue.")),

		// Backup history charts.
		sized(largePanelWidth, largePanelHeight, timeseriesPanel("Backup runs by outcome", units.Number,
			query(fmt.Sprintf(
				"round(sum by(cluster_name, outcome) (increase(klio_plugin_backup_runs_total{%s}[$__range])), 1)",
				clientMatcher), "{{cluster_name}} / {{outcome}}"),
		).Description("Base backup runs, grouped by cluster and outcome (success or failure).")),

		sized(largePanelWidth, largePanelHeight, timeseriesPanel("Failed backup runs by reason", units.Number,
			query(fmt.Sprintf(
				"round(sum by (cluster_name, failure_category) "+
					"(increase(klio_plugin_backup_runs_total{outcome=\"failure\",%s}[$__range])), 1)", clientMatcher),
				"{{cluster_name}} / {{failure_category}}",
			),
		).Description("Failed base backup runs, grouped by cluster and failure reason.")),

		sized(largePanelWidth, largePanelHeight, timeseriesPanel("Backup duration percentiles", units.Seconds,
			quantileTargetsAbsolute("klio_plugin_backup_duration_seconds_bucket", "le, cluster_name",
				"outcome=\"success\","+clientMatcher, "{{cluster_name}}")...,
		).Description("50th/90th/99th-percentile duration of successful base backup runs, per cluster.")),

		// WAL streaming.
		sized(largePanelWidth, mediumPanelHeight, timelinePanel("WAL streaming timeline",
			query(fmt.Sprintf("max by (cluster_name) (klio_client_wal_timeline{%s})", clientMatcher),
				"{{cluster_name}}"),
		).Description("PostgreSQL timeline of the WAL streaming client.")),

		sized(largePanelWidth, largePanelHeight,
			timeseriesPanel("WAL block send duration percentiles (rate)", units.Nanoseconds,
				quantileTargets("klio_client_wal_block_duration_nanoseconds_bucket", "le, cluster_name",
					clientMatcher, "{{cluster_name}}")...,
			).Description("50th/90th/99th-percentile duration of the client's gRPC send of a WAL block to the "+
				"server, per cluster. Reflects recent activity, over a rolling few-minute window.")),

		sized(largePanelWidth, largePanelHeight,
			timeseriesPanel("WAL block send duration percentiles (total)", units.Nanoseconds,
				quantileTargetsAbsolute("klio_client_wal_block_duration_nanoseconds_bucket", "le, cluster_name",
					clientMatcher, "{{cluster_name}}")...,
			).Description("50th/90th/99th-percentile duration of the client's gRPC send of a WAL block to the "+
				"server, per cluster. Reflects all activity since the server last restarted.")),

		// WAL restore. Every RESTORE_WAL request the plugin serves to PostgreSQL
		// records klio.plugin.wal.restore_duration. The percentiles keep only
		// successful restores and split on cache_hit: a prefetch hit is a local
		// rename and a miss waits on a download, so pooled together the
		// percentiles would drift with the hit ratio instead of describing
		// either case.
		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL restore duration percentiles (rate)", units.Nanoseconds,
				quantileTargets("klio_plugin_wal_restore_duration_nanoseconds_bucket", "le, cluster_name, cache_hit",
					"outcome=\"success\","+clientMatcher, "{{cluster_name}} / cache_hit={{cache_hit}}")...,
			).Description("50th/90th/99th-percentile end-to-end duration of successful WAL restores served by the "+
				"plugin to PostgreSQL, per cluster, split by whether the WAL was already in the prefetch spool "+
				"(cache_hit=true) or had to be downloaded (cache_hit=false). Reflects recent activity, over the "+
				"rate interval window.")),

		sized(largestPanelWidth, largePanelHeight,
			timeseriesPanel("WAL restore duration percentiles (total)", units.Nanoseconds,
				quantileTargetsAbsolute("klio_plugin_wal_restore_duration_nanoseconds_bucket",
					"le, cluster_name, cache_hit", "outcome=\"success\","+clientMatcher,
					"{{cluster_name}} / cache_hit={{cache_hit}}")...,
			).Description("50th/90th/99th-percentile end-to-end duration of successful WAL restores served by the "+
				"plugin to PostgreSQL, per cluster, split by whether the WAL was already in the prefetch spool "+
				"(cache_hit=true) or had to be downloaded (cache_hit=false). Reflects all activity since the "+
				"plugin sidecar last restarted.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("WAL restores (rate)", units.OpsPerSecond,
			query(fmt.Sprintf("sum by (cluster_name, tier, outcome) "+
				"(rate(klio_plugin_wal_restore_duration_nanoseconds_count{%s}[$__rate_interval]))", clientMatcher),
				"{{cluster_name}} {{tier}} / {{outcome}}"),
		).Description("Rate of WAL restores served by the plugin to PostgreSQL, per cluster, split by the tier "+
			"that served them and by outcome. not_found is PostgreSQL asking for a WAL that was never archived, "+
			"the routine end-of-archive signal rather than an error.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("WAL restores (total)", units.Number,
			query(fmt.Sprintf("sum by (cluster_name, tier, outcome) "+
				"(klio_plugin_wal_restore_duration_nanoseconds_count{%s})", clientMatcher),
				"{{cluster_name}} {{tier}} / {{outcome}}"),
		).Description("Total WAL restores served by the plugin to PostgreSQL since the plugin sidecar last "+
			"restarted, per cluster, split by the tier that served them and by outcome.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("WAL restore prefetch hit ratio", units.PercentUnit,
			query(fmt.Sprintf("sum by (cluster_name) (rate(klio_plugin_wal_restore_duration_nanoseconds_count"+
				"{outcome=\"success\",cache_hit=\"true\",%s}[$__rate_interval])) / "+
				"sum by (cluster_name) (rate(klio_plugin_wal_restore_duration_nanoseconds_count"+
				"{outcome=\"success\",%s}[$__rate_interval]))", clientMatcher, clientMatcher),
				"{{cluster_name}}"),
		).Description("Fraction of successful WAL restores served straight from the prefetch spool instead of "+
			"waiting on a download, per cluster. A falling ratio means prefetch is not keeping pace with "+
			"PostgreSQL replay.")),
	}
}
