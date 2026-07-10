package main

import (
	"fmt"
)

// clientPanels returns the "Client / Plugin" section panels. These metrics are
// emitted by the Klio plugin sidecar that runs in each PostgreSQL pod (the
// `klio.plugin.backup.*` family, exported to Prometheus as
// `klio_plugin_backup_*`). Every query is scoped by the $namespace variable.
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
		// Backup volume over time: count of runs per bucket, split by outcome.
		// Bars aggregate how many backups happened, which is more useful for
		// infrequent backups than instantaneous duration percentiles.
		sized(8, panelHeight, barPanel("Backups by outcome", "short",
			query(
				fmt.Sprintf("sum by (outcome) (increase(klio_plugin_backup_runs_total{%s}[$__interval]))", nsMatcher),
				"{{outcome}}"),
		).Description("Count of base backup runs per time bucket, split by outcome. Bars aggregate the "+
			"number of backups over each interval (per day on a multi-day range).")),
	}
}
