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

// Command grafana generates the Klio Grafana dashboard from code using the
// grafana-foundation-sdk. The dashboard is built against the Prometheus names
// of the OpenTelemetry metrics that Klio exports (see
// documentation/web/docs/user/opentelemetry.md) and is split into row
// sections: one for the client (plugin sidecar and the WAL streaming client
// it supervises) metrics, one for the server metrics (including the retained
// PostgreSQL backups), and one for WAL replication lag (sourced from
// CloudNativePG's pg_stat_replication metrics).
//
// Running the command regenerates the committed JSON in place:
//
//	go run . -output klio-dashboard.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/cog/variants"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// lsnHalf is 2^32: a PostgreSQL LSN is a 64-bit byte offset that PostgreSQL
// prints as two 32-bit halves in hexadecimal separated by a slash (e.g.
// `16/B374D848`). Dividing by lsnHalf yields the high half, the remainder is
// the low half.
const lsnHalf = 4294967296

const (
	// datasourceVar is the name of the Prometheus data source template
	// variable that every panel queries through, so the dashboard is portable
	// across Grafana installations.
	datasourceVar = "datasource"

	// panelHeight is the shared grid height (in rows) of every panel, so the
	// dashboard grid packs flush without vertical gaps.
	panelHeight = 6

	// clientMatcher selects the plugin sidecar's per-cluster series
	// (klio_plugin_backup_* and klio_client_wal_*). These carry the PostgreSQL
	// pod's k8s.namespace.name and a cluster_name, so they are scoped by
	// $namespace and $cluster. Because they identify their cluster directly,
	// several clusters sharing one namespace never fold into a single series.
	clientMatcher = `k8s_namespace_name=~"$namespace",cluster_name=~"$cluster"`
	// serverMatcher selects the server-level klio_server_* series that carry no
	// cluster_name (uptime, the embedded queue, verifications and the Kopia base
	// snapshots) by the Klio server's OpenTelemetry service.name. service.name
	// identifies a server uniquely even when two servers share a pod host name
	// (host_name collides when two Servers have the same name in different
	// namespaces); k8s.namespace.name is deliberately NOT used here because a
	// server tags every series with its OWN namespace, which would hide the
	// metrics of a cluster it serves cross-namespace.
	serverMatcher = `service_name=~"$server"`
	// walMatcher additionally selects on cluster_name, which the per-cluster
	// klio_server_wal_* and klio_server_backup_* (backups / latest_backup_* /
	// oldest_backup_*) series carry, and combines it with the server selector.
	walMatcher = `service_name=~"$server",cluster_name=~"$cluster"`
)

func main() {
	output := flag.String("output", "klio-dashboard.json", "path of the dashboard JSON file to write")
	flag.Parse()

	builder := build()

	dash, err := builder.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "building dashboard: %v\n", err)
		os.Exit(1)
	}

	encoded, err := json.MarshalIndent(dash, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshaling dashboard: %v\n", err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')

	if err := os.WriteFile(*output, encoded, 0o644); err != nil { //nolint:gosec // world-readable dashboard JSON is fine
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", *output, err)
		os.Exit(1)
	}
}

// sizedPanel is a panel with its grid footprint, ready to be positioned by
// layoutSection.
type sizedPanel struct {
	w, h  int
	place func(dashboard.GridPos) cog.Builder[dashboard.Panel]
}

// sized pairs a panel builder (stat, timeseries, ...) with its grid
// width and height so layoutSection can assign it an explicit position.
func sized[B interface {
	GridPos(gridPos dashboard.GridPos) B
	cog.Builder[dashboard.Panel]
}](w, h int, b B) sizedPanel {
	return sizedPanel{w: w, h: h, place: func(pos dashboard.GridPos) cog.Builder[dashboard.Panel] {
		return b.GridPos(pos)
	}}
}

// datasourceRef returns a reference to the dashboard's Prometheus data source
// variable, used by every panel and query.
func datasourceRef() common.DataSourceRef {
	dsType := "prometheus"
	uid := fmt.Sprintf("${%s}", datasourceVar)
	return common.DataSourceRef{Type: &dsType, Uid: &uid}
}

// query builds a ranged Prometheus query target with a legend.
func query(expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().
		Datasource(datasourceRef()).
		Expr(expr).
		LegendFormat(legend).
		Range()
}

// instantTableTarget builds an instant Prometheus query in table format, used
// by the LSN tables where only the current value per cluster/tier matters.
func instantTableTarget(expr, refID string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().
		Datasource(datasourceRef()).
		Expr(expr).
		Format(prometheus.PromQueryFormatTable).
		Instant().
		RefId(refID)
}

// lsnColumn is one LSN value rendered as a pair of hexadecimal columns (high
// and low 32-bit halves) in an lsnTablePanel. agg is the PromQL that reduces
// the LSN metric to one series per cluster_name and tier.
type lsnColumn struct {
	label string
	agg   string
}

// lsnTablePanel renders one or more PostgreSQL LSNs per cluster and tier as a
// table, splitting each 64-bit byte offset into its high and low 32-bit halves
// shown in hexadecimal (unit "hex"). Read together the two columns are
// PostgreSQL's own X/Y LSN notation (high / low): Grafana cannot join them into
// a single "X/Y" string from a numeric series, and rendering the whole offset
// as one hex value drops the half boundary, so the halves are split in PromQL
// (floor(lsn/2^32) and lsn%2^32) and shown side by side. A byte-size unit would
// be wrong: an LSN is a position, not an amount of data.
func lsnTablePanel(title string, cols ...lsnColumn) *table.PanelBuilder {
	// Two label columns (cluster, tier) plus a hi/lo pair per LSN. Fix a column
	// width that keeps every hi/lo half visible inside the Span(8) panel instead
	// of letting Grafana auto-size them wide enough to push columns off-screen.
	colWidth := 470.0 / float64(2+2*len(cols))
	panel := table.NewPanelBuilder().
		Title(title).
		Datasource(datasourceRef()).
		Unit("hex").
		Decimals(0).
		Width(colWidth).
		Span(8).
		Height(panelHeight)

	// organize transform: drop the Time column, label the cluster/tier columns
	// and order everything, renaming each query's "Value #<refID>" field.
	exclude := map[string]any{"Time": true}
	rename := map[string]any{"cluster_name": "cluster", "tier": "tier"}
	order := map[string]any{"cluster_name": 0, "tier": 1}
	idx := 2
	for i, c := range cols {
		hiRef := fmt.Sprintf("h%d", i)
		loRef := fmt.Sprintf("l%d", i)
		panel = panel.
			WithTarget(instantTableTarget(fmt.Sprintf("floor(%s / %d)", c.agg, lsnHalf), hiRef)).
			WithTarget(instantTableTarget(fmt.Sprintf("%s %% %d", c.agg, lsnHalf), loRef))
		rename["Value #"+hiRef] = c.label + " (hi)"
		rename["Value #"+loRef] = c.label + " (lo)"
		order["Value #"+hiRef] = idx
		order["Value #"+loRef] = idx + 1
		idx += 2
	}

	return panel.
		WithTransformation(dashboard.DataTransformerConfig{
			Id:      "merge",
			Options: map[string]any{},
		}).
		WithTransformation(dashboard.DataTransformerConfig{
			Id: "organize",
			Options: map[string]any{
				"excludeByName": exclude,
				"renameByName":  rename,
				"indexByName":   order,
			},
		})
}

// quantiles are the percentiles every latency panel renders together, so the
// median, the tail and the extreme tail always appear side by side rather than
// a single percentile hiding the shape of the distribution.
//
//nolint:gochecknoglobals
var quantiles = []struct {
	q     float64
	label string
}{
	{0.50, "p50"},
	{0.95, "p95"},
	{0.99, "p99"},
}

// quantileTargetsWindow builds one p50/p95/p99 target for a histogram, applying
// counterFn (`rate` or `increase`) over window (e.g. `$__rate_interval` or
// `$__range`). It groups the _bucket series by groupBy (which MUST include
// `le`, or histogram_quantile cannot find the bucket boundaries and the panel
// renders no data) and labels each series "<pN> <legend>", dropping the
// trailing space when legend is empty.
func quantileTargetsWindow(
	bucketMetric, groupBy, matcher, legend, counterFn, window string,
) []cog.Builder[variants.Dataquery] {
	targets := make([]cog.Builder[variants.Dataquery], 0, len(quantiles))
	for _, p := range quantiles {
		targets = append(targets, query(
			fmt.Sprintf("histogram_quantile(%.2f, sum by (%s) (%s(%s{%s}[%s])))",
				p.q, groupBy, counterFn, bucketMetric, matcher, window),
			strings.TrimSpace(p.label+" "+legend)))
	}

	return targets
}

// quantileTargets builds p50/p95/p99 targets for a high-frequency histogram,
// using rate() over $__rate_interval so the percentiles track the dashboard's
// selected range and zoom.
func quantileTargets(bucketMetric, groupBy, matcher, legend string) []cog.Builder[variants.Dataquery] {
	return quantileTargetsWindow(bucketMetric, groupBy, matcher, legend, "rate", "$__rate_interval")
}

// tableLegend renders a compact table legend at the bottom of a panel.
func tableLegend() *common.VizLegendOptionsBuilder {
	return common.NewVizLegendOptionsBuilder().
		ShowLegend(true).
		DisplayMode(common.LegendDisplayModeList).
		Placement(common.LegendPlacementBottom)
}

// timeseriesPanel builds a timeseries panel with the given title, unit and
// query targets, wired to the dashboard data source.
func timeseriesPanel(title, unit string, targets ...cog.Builder[variants.Dataquery]) *timeseries.PanelBuilder {
	panel := timeseries.NewPanelBuilder().
		Title(title).
		Datasource(datasourceRef()).
		Unit(unit).
		FillOpacity(10).
		GradientMode(common.GraphGradientModeOpacity).
		Legend(tableLegend()).
		// 8/24 columns => three timeseries per row for a dense layout.
		Span(8).
		// Uniform height across stat and timeseries panels so the grid packs
		// flush with no vertical gaps (the auto-layout starts each new row below
		// the tallest panel of the previous one).
		Height(panelHeight)
	for _, target := range targets {
		panel = panel.WithTarget(target)
	}

	return panel
}

// timelinePanel renders an integer PostgreSQL timeline ID over time as a
// stepped line, so an operator sees not only the current timeline but exactly
// when it changed (a promotion or failover), which a single current value (a
// stat or bar gauge) cannot convey. The unit is a plain count with no decimals.
func timelinePanel(title string, targets ...cog.Builder[variants.Dataquery]) *timeseries.PanelBuilder {
	return timeseriesPanel(title, "short", targets...).
		FillOpacity(0).
		LineInterpolation(common.LineInterpolationStepAfter).
		LineWidth(2).
		Decimals(0)
}

// barPanel builds a stacked bar chart (a timeseries in bar draw style) for
// counting discrete events over time, such as backups per bucket.
func barPanel(title, unit string, targets ...cog.Builder[variants.Dataquery]) *timeseries.PanelBuilder {
	return timeseriesPanel(title, unit, targets...).
		DrawStyle(common.GraphDrawStyleBars).
		FillOpacity(80).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Decimals(0)
}

// statPanel builds a stat panel showing the last value of its query targets,
// wired to the dashboard data source.
func statPanel(title, unit string, targets ...cog.Builder[variants.Dataquery]) *stat.PanelBuilder {
	panel := stat.NewPanelBuilder().
		Title(title).
		Datasource(datasourceRef()).
		Unit(unit).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"})).
		GraphMode(common.BigValueGraphModeNone).
		// Color the value using Grafana's classic palette (one color per
		// series) rather than threshold coloring, so red stays reserved for
		// real alert/risk panels (none defined yet).
		ColorMode(common.BigValueColorModeValue).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		// Cap the value/title font size: Grafana's auto-sizer otherwise blows
		// long strings (e.g. the "21 minutes" duration format) up until they
		// overflow and clip in the dense tiles.
		Text(common.NewVizTextDisplayOptionsBuilder().TitleSize(14).ValueSize(28)).
		// 4/24 columns => six stat tiles per row for a dense layout.
		Span(4).
		Height(panelHeight)
	for _, target := range targets {
		panel = panel.WithTarget(target)
	}

	return panel
}
