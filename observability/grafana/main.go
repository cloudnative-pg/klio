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
	"github.com/grafana/grafana-foundation-sdk/go/text"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
	"github.com/grafana/grafana-foundation-sdk/go/units"
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

	// mediumPanelHeight is the shared grid height (in rows) of every panel, so the
	// dashboard grid packs flush without vertical gaps.
	mediumPanelHeight = 6
	largePanelHeight  = 8
	// descriptionPanelHeight is the grid height (in rows) of the text panel
	// that explains each row section.
	descriptionPanelHeight = 2

	gridWidth          = 24
	smallestPanelWidth = 3
	smallPanelWidth    = 4
	mediumPanelWidth   = 6
	largePanelWidth    = 8
	largestPanelWidth  = 12

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
// table. Each 64-bit byte offset is split into its high and low 32-bit halves
// in PromQL (floor(lsn/2^32) and lsn%2^32, hidden targets h<i>/l<i>), then a
// Grafana SQL expression (the __expr__ datasource) joins the halves on
// cluster_name/tier and renders PostgreSQL's own X/Y hex notation as a single
// visible column per LSN. Neither PromQL nor a Grafana transform can do this
// join+format: PromQL has no decimal-to-hex formatting, and no transform
// combines two numeric fields into one templated string. __value__ is the
// numeric column exposed for an instant/table-format query result.
func lsnTablePanel(title string, cols ...lsnColumn) *table.PanelBuilder {
	panel := table.NewPanelBuilder().
		Title(title).
		Datasource(datasourceRef()).
		Span(8).
		Height(mediumPanelHeight)

	joins := []string{"JOIN l0 ON l0.cluster_name = h0.cluster_name AND l0.tier = h0.tier"}
	columns := []string{
		`h0.cluster_name AS "Cluster Name"`,
		`h0.tier AS "Tier"`,
		fmt.Sprintf(`concat(CONV(h0.__value__, 10, 16), "/", CONV(l0.__value__, 10, 16)) as "%s LSN"`,
			cols[0].label),
	}
	for i, c := range cols {
		hiRef := fmt.Sprintf("h%d", i)
		loRef := fmt.Sprintf("l%d", i)
		panel = panel.
			WithTarget(instantTableTarget(fmt.Sprintf("floor(%s / %d)", c.agg, lsnHalf), hiRef).Hide(true)).
			WithTarget(instantTableTarget(fmt.Sprintf("%s %% %d", c.agg, lsnHalf), loRef).Hide(true))
		if i == 0 {
			continue
		}
		joins = append(joins,
			fmt.Sprintf("JOIN %s ON %s.cluster_name = h0.cluster_name AND %s.tier = h0.tier", hiRef, hiRef, hiRef),
			fmt.Sprintf("JOIN %s ON %s.cluster_name = h0.cluster_name AND %s.tier = h0.tier", loRef, loRef, loRef))
		columns = append(columns, fmt.Sprintf(
			`concat(CONV(%s.__value__, 10, 16), "/", CONV(%s.__value__, 10, 16)) as "%s LSN"`,
			hiRef, loRef, c.label))
	}
	sql := fmt.Sprintf("SELECT\n  %s\nFROM h0\n%s", strings.Join(columns, ",\n  "), strings.Join(joins, "\n"))

	return panel.WithTarget(variants.NewUnknownDataqueryBuilderFromObject(variants.UnknownDataquery{
		"refId":      "sql",
		"hide":       false,
		"datasource": map[string]any{"type": "__expr__", "uid": "__expr__"},
		"type":       "sql",
		"expression": sql,
	}))
}

// quantileSpec is one percentile rendered by a latency panel: the quantile
// value passed to histogram_quantile and the legend label for its series.
type quantileSpec struct {
	q     float64
	label string
}

// quantiles are the percentiles every latency panel renders together, so the
// median, the tail and the extreme tail always appear side by side rather
// than a single percentile hiding the shape of the distribution.
//
//nolint:gochecknoglobals
var quantiles = []quantileSpec{
	{0.50, "p50"},
	{0.90, "p90"},
	{0.99, "p99"},
}

// quantileTargetsWindow builds one target per entry in quantiles for a
// histogram, applying counterFn (`rate` or `increase`) over window (e.g.
// `$__rate_interval` or `$__range`). It groups the _bucket series by groupBy
// (which MUST include `le`, or histogram_quantile cannot find the bucket
// boundaries and the panel renders no data) and labels each series
// "<legend> <pN>", dropping the leading space when legend is empty.
func quantileTargetsWindow(
	bucketMetric, groupBy, matcher, legend, counterFn, window string,
) []cog.Builder[variants.Dataquery] {
	targets := make([]cog.Builder[variants.Dataquery], 0, len(quantiles))
	for _, p := range quantiles {
		targets = append(targets, query(
			fmt.Sprintf("histogram_quantile(%.2f, sum by (%s) (%s(%s{%s}[%s])))",
				p.q, groupBy, counterFn, bucketMetric, matcher, window),
			strings.TrimSpace(legend+" "+p.label)))
	}

	return targets
}

// quantileTargets builds one target per entry in quantiles for a
// high-frequency histogram, using rate() over $__rate_interval so the
// percentiles track the dashboard's selected range and zoom.
func quantileTargets(bucketMetric, groupBy, matcher, legend string) []cog.Builder[variants.Dataquery] {
	return quantileTargetsWindow(bucketMetric, groupBy, matcher, legend, "rate", "$__rate_interval")
}

// quantileTargetsAbsolute builds one target per entry in quantiles straight
// off the raw cumulative bucket counters (no rate()/increase()), so a single
// observation still yields a value instead of NaN. The tradeoff: each point
// is a since-restart quantile that dilutes as more observations accrue and
// resets on a server restart, rather than one scoped to the dashboard's
// selected range or zoom.
func quantileTargetsAbsolute(bucketMetric, groupBy, matcher, legend string) []cog.Builder[variants.Dataquery] {
	targets := make([]cog.Builder[variants.Dataquery], 0, len(quantiles))
	for _, p := range quantiles {
		targets = append(targets, query(
			fmt.Sprintf("histogram_quantile(%.2f, sum by (%s) (%s{%s}))", p.q, groupBy, bucketMetric, matcher),
			strings.TrimSpace(legend+" "+p.label)))
	}

	return targets
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
		Span(largePanelWidth).
		// Uniform height across stat and timeseries panels so the grid packs
		// flush with no vertical gaps (the auto-layout starts each new row below
		// the tallest panel of the previous one).
		Height(mediumPanelHeight)
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
	return timeseriesPanel(title, units.Number, targets...).
		FillOpacity(0).
		LineInterpolation(common.LineInterpolationStepAfter).
		LineWidth(2).
		Decimals(0)
}

// descriptionPanel builds a transparent markdown text panel that explains a
// row section (a Grafana row has no description field of its own).
func descriptionPanel(content string) *text.PanelBuilder {
	return text.NewPanelBuilder().
		Transparent(true).
		Mode(text.TextModeMarkdown).
		Content(content)
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
		Span(smallPanelWidth).
		Height(mediumPanelHeight)
	for _, target := range targets {
		panel = panel.WithTarget(target)
	}

	return panel
}
