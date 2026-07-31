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

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/cog/variants"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

const (
	// datasourceVar is the name of the Prometheus data source template
	// variable that every panel queries through, so the dashboard is portable
	// across Grafana installations.
	datasourceVar = "datasource"

	// panelHeight is the shared grid height (in rows) of every panel, so the
	// dashboard grid packs flush without vertical gaps.
	panelHeight = 6

	// nsMatcher is the namespace label selector every klio metric carries
	// (the Prometheus export of the k8s.namespace.name resource attribute).
	nsMatcher = `k8s_namespace_name=~"$namespace"`
	// serverMatcher additionally selects on host_name, which the klio_server_*
	// series carry as the Klio server host (the StatefulSet pod), and combines
	// it with the namespace selector.
	serverMatcher = `k8s_namespace_name=~"$namespace",host_name=~"$server"`
	// walMatcher additionally selects on cluster_name, which only the
	// klio_server_wal_* series carry, and combines it with the namespace and
	// server selectors.
	walMatcher = `k8s_namespace_name=~"$namespace",host_name=~"$server",cluster_name=~"$cluster"`
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

// sized pairs a panel builder (stat, timeseries, bargauge, ...) with its grid
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

// quantileTargetsRange builds p50/p95/p99 targets for an infrequent-event
// histogram (e.g. backups), using increase() over the whole visible range
// ($__range). A short rate() window almost never catches a rare event, so the
// percentiles would otherwise collapse to no data except at the instant the
// event fires; the range window keeps them populated whenever the selected
// range spans at least one event.
func quantileTargetsRange(bucketMetric, groupBy, matcher, legend string) []cog.Builder[variants.Dataquery] {
	return quantileTargetsWindow(bucketMetric, groupBy, matcher, legend, "increase", "$__range")
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

// barPanel builds a stacked bar chart (a timeseries in bar draw style) for
// counting discrete events over time, such as backups per bucket.
func barPanel(title, unit string, targets ...cog.Builder[variants.Dataquery]) *timeseries.PanelBuilder {
	return timeseriesPanel(title, unit, targets...).
		DrawStyle(common.GraphDrawStyleBars).
		FillOpacity(80).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Decimals(0)
}

// barGaugePanel builds a horizontal bar gauge that renders one labeled bar per
// series, so multi-series "by tier / by stream" values always show every series
// (a narrow stat tile can hide all but the first). Every caller renders a
// label-only classification (tier, cluster, stream), so the unit is always
// "none".
func barGaugePanel(title string, targets ...cog.Builder[variants.Dataquery]) *bargauge.PanelBuilder {
	panel := bargauge.NewPanelBuilder().
		Title(title).
		Datasource(datasourceRef()).
		Unit("none").
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"})).
		Orientation(common.VizOrientationHorizontal).
		Decimals(0)
	for _, target := range targets {
		panel = panel.WithTarget(target)
	}

	return panel
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
