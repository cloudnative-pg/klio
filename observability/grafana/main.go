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
// grafana-foundation-sdk, against the Prometheus names of the OpenTelemetry
// metrics that Klio exports (see documentation/web/docs/user/opentelemetry.md).
// The dashboard uses the schema v2 (Grafana 13.1 or later): one top-level
// row is organized by PostgreSQL cluster, the other by Klio server, each with
// its own variables.
//
// Running the command regenerates the committed JSON in place:
//
//	go run . -output dashboards/klio.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2beta1"
	"github.com/grafana/grafana-foundation-sdk/go/expr"
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
	// tablePanelHeight fits a table with one row per cluster and tier.
	tablePanelHeight = 10
	// descriptionPanelHeight is the grid height (in rows) of the text panel
	// that explains each row section.
	descriptionPanelHeight = 2

	gridWidth          = 24
	smallestPanelWidth = 3
	smallPanelWidth    = 4
	mediumPanelWidth   = 6
	largePanelWidth    = 8
	largestPanelWidth  = 12

	// clientMatcher selects the plugin sidecar series (klio_plugin_* and
	// klio_client_wal_*), which carry the PostgreSQL pod's namespace, cluster
	// and pod name.
	clientMatcher = `k8s_namespace_name=~"$namespace",cluster_name=~"$cluster",k8s_pod_name=~"$pod"`
	// clusterMatcher selects the per-cluster server series by cluster_name
	// alone: they carry the server's namespace, not the cluster's.
	clusterMatcher = `cluster_name=~"$cluster"`
	// serverMatcher selects the server-level series by the Klio server's
	// namespace and name: a Server's pod is named `<server>-klio-<ordinal>`.
	serverMatcher = `k8s_namespace_name=~"$server_namespace",k8s_pod_name=~"($server)-klio-[0-9]+"`
	// walMatcher selects the per-cluster server series by server and served
	// cluster.
	walMatcher = serverMatcher + `,cluster_name=~"$served_cluster"`
	// snapshotMatcher selects the Kopia snapshots by server, and by served
	// cluster through snapshot_source (`userName@clusterName:path`).
	snapshotMatcher = serverMatcher + `,snapshot_source=~"[^@]*@$served_cluster:.*"`

	// clusterJoin narrows server series, which carry neither the cluster's
	// namespace nor its instances, to the clusters whose plugin series match
	// every variable of the Clusters group.
	clusterJoin = `and on (cluster_name) group by (cluster_name) (klio_plugin_backup_in_progress{` +
		clientMatcher + `})`
	// servedClusterJoin narrows server-level series, which carry no
	// cluster_name, to the servers with series of the selected served
	// clusters. Its "All" matches series without cluster_name too, so no
	// server is hidden until served clusters are selected.
	servedClusterJoin = `and on (k8s_namespace_name, k8s_pod_name) group by (k8s_namespace_name, k8s_pod_name) ` +
		`({__name__=~"klio_server_.+",` + walMatcher + `})`
)

// scope selects the series of a metric: the label matchers of its
// selector, and an optional join that narrows the result on labels the
// series do not carry.
type scope struct {
	matcher string
	join    string
}

//nolint:gochecknoglobals
var (
	// clusterScope selects the server series on the Clusters group.
	clusterScope = scope{matcher: clusterMatcher, join: clusterJoin}
	// servedScope selects the per-cluster server series on the Servers group.
	servedScope = scope{matcher: walMatcher}
	// serverScope selects the server-level series on the Servers group.
	serverScope = scope{matcher: serverMatcher, join: servedClusterJoin}
)

// narrow applies the join of the scope to an instant vector expression.
func (s scope) narrow(expr string) string {
	if s.join == "" {
		return expr
	}

	return "(" + expr + " " + s.join + ")"
}

// series selects the series of a metric in the scope.
func (s scope) series(metric string) string {
	return s.narrow(metric + "{" + s.matcher + "}")
}

// over applies a range function (rate, increase) over window to the series
// of a metric in the scope. The join applies after it, since it only takes
// instant vectors.
func (s scope) over(fn, metric, window string) string {
	return s.narrow(fmt.Sprintf("%s(%s{%s}[%s])", fn, metric, s.matcher, window))
}

func main() {
	output := flag.String("output", "dashboards/klio.json", "path of the dashboard JSON file to write")
	flag.Parse()

	dash, err := build().Build()
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

// panel is a schema v2 panel being built: the fields of the panel itself, its
// queries and its visualization.
type panel struct {
	title       string
	description string
	transparent bool
	targets     []target
	viz         cog.Builder[dashboardv2beta1.VizConfigKind]
}

// target is a panel query, with the refId it is given unless refID is set.
type target struct {
	query  cog.Builder[dashboardv2.DataQueryKind]
	refID  string
	hidden bool
}

// build returns the panel element with the given ID.
func (p *panel) build(id int) (dashboardv2.PanelKind, error) {
	queries := dashboardv2.NewQueryGroupBuilder()
	for i, t := range p.targets {
		refID := t.refID
		if refID == "" {
			refID = fmt.Sprintf("%c", 'A'+i)
		}
		queries.Target(dashboardv2.NewTargetBuilder().Query(t.query).RefId(refID).Hidden(t.hidden))
	}
	builder := dashboardv2.NewPanelBuilder().
		Id(float64(id)).
		Title(p.title).
		Links([]cog.Builder[dashboardv2.DataLink]{}).
		Data(queries).
		Visualization(vizConfig{p.viz})
	if p.description != "" {
		builder.Description(p.description)
	}
	if p.transparent {
		builder.Transparent(true)
	}

	return builder.Build()
}

// statPanelBuilder, timeseriesPanelBuilder, tablePanelBuilder and
// textPanelBuilder pair a panel with its typed visualization builder, and
// offer the few visualization settings the panels set on top of the helpers'
// defaults.
type (
	statPanelBuilder struct {
		panel

		vis *stat.VisualizationBuilder
	}
	timeseriesPanelBuilder struct {
		panel

		vis *timeseries.VisualizationBuilder
	}
	tablePanelBuilder struct {
		panel
	}
	textPanelBuilder struct {
		panel
	}
)

// Description sets the panel description, shown in its info tooltip.
func (b *statPanelBuilder) Description(description string) *statPanelBuilder {
	b.description = description
	return b
}

// Decimals sets the number of decimals of the values.
func (b *statPanelBuilder) Decimals(decimals float64) *statPanelBuilder {
	b.vis.Decimals(decimals)
	return b
}

// Orientation sets how the stat tiles are stacked.
func (b *statPanelBuilder) Orientation(orientation common.VizOrientation) *statPanelBuilder {
	b.vis.Orientation(orientation)
	return b
}

// Description sets the panel description, shown in its info tooltip.
func (b *timeseriesPanelBuilder) Description(description string) *timeseriesPanelBuilder {
	b.description = description
	return b
}

// Description sets the panel description, shown in its info tooltip.
func (b *tablePanelBuilder) Description(description string) *tablePanelBuilder {
	b.description = description
	return b
}

func (b *statPanelBuilder) base() *panel       { return &b.panel }
func (b *timeseriesPanelBuilder) base() *panel { return &b.panel }
func (b *tablePanelBuilder) base() *panel      { return &b.panel }
func (b *textPanelBuilder) base() *panel       { return &b.panel }

// sizedPanel is a panel with its grid footprint, ready to be positioned by
// pack.
type sizedPanel struct {
	w, h  int
	panel *panel
}

// sized pairs a panel with its grid width and height so pack can assign it
// an explicit position.
func sized(w, h int, b interface{ base() *panel }) sizedPanel {
	return sizedPanel{w: w, h: h, panel: b.base()}
}

// datasourceRef returns a reference to the dashboard's Prometheus data source
// variable, used by every query.
func datasourceRef() cog.Builder[dashboardv2.Dashboardv2DataQueryKindDatasource] {
	return built[dashboardv2.Dashboardv2DataQueryKindDatasource]{
		dashboardv2.Dashboardv2DataQueryKindDatasource{Name: cog.ToPtr(datasourceRefUID())},
	}
}

// datasourceRefUID is the data source variable as a data source UID.
func datasourceRefUID() string {
	return fmt.Sprintf("${%s}", datasourceVar)
}

// query builds a ranged Prometheus query target with a legend.
func query(expr, legend string) target {
	return target{query: prometheus.NewQueryV2Builder().
		Version("v0").
		Datasource(datasourceRef()).
		Expr(expr).
		LegendFormat(legend).
		Range(true)}
}

// instantTableTarget builds a hidden instant Prometheus query in table format,
// used by the LSN tables where only the current value per cluster/tier
// matters.
func instantTableTarget(expr, refID string) target {
	return target{query: prometheus.NewQueryV2Builder().
		Version("v0").
		Datasource(datasourceRef()).
		Expr(expr).
		Format(prometheus.PromQueryFormatTable).
		Instant(true), refID: refID, hidden: true}
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
func lsnTablePanel(title string, cols ...lsnColumn) *tablePanelBuilder {
	b := &tablePanelBuilder{panel{title: title, viz: table.NewVisualizationBuilder()}}

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
		b.targets = append(b.targets,
			instantTableTarget(fmt.Sprintf("floor(%s / %d)", c.agg, lsnHalf), hiRef),
			instantTableTarget(fmt.Sprintf("%s %% %d", c.agg, lsnHalf), loRef))
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

	b.targets = append(b.targets, target{query: expr.NewQueryV2Builder().
		Version("v0").
		Datasource(built[dashboardv2.Dashboardv2DataQueryKindDatasource]{
			dashboardv2.Dashboardv2DataQueryKindDatasource{Name: cog.ToPtr("__expr__")},
		}).
		Sql(expr.NewTypeSqlBuilder().Expression(sql)), refID: "sql"})

	return b
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
func quantileTargetsWindow(bucketMetric, groupBy, matcher, legend, counterFn, window string) []target {
	return quantileTargetsWindowIn(bucketMetric, groupBy, scope{matcher: matcher}, legend, counterFn, window)
}

// quantileTargetsWindowIn is quantileTargetsWindow for the series of a
// scope.
func quantileTargetsWindowIn(bucketMetric, groupBy string, sc scope, legend, counterFn, window string) []target {
	targets := make([]target, 0, len(quantiles))
	for _, p := range quantiles {
		targets = append(targets, query(
			fmt.Sprintf("histogram_quantile(%.2f, sum by (%s) (%s))",
				p.q, groupBy, sc.over(counterFn, bucketMetric, window)),
			strings.TrimSpace(legend+" "+p.label)))
	}

	return targets
}

// quantileTargets builds one target per entry in quantiles for a
// high-frequency histogram, using rate() over $__rate_interval so the
// percentiles track the dashboard's selected range and zoom.
func quantileTargets(bucketMetric, groupBy, matcher, legend string) []target {
	return quantileTargetsWindow(bucketMetric, groupBy, matcher, legend, "rate", "$__rate_interval")
}

// quantileTargetsAbsolute builds one target per entry in quantiles straight
// off the raw cumulative bucket counters (no rate()/increase()), so a single
// observation still yields a value instead of NaN. The tradeoff: each point
// is a since-restart quantile that dilutes as more observations accrue and
// resets on a server restart, rather than one scoped to the dashboard's
// selected range or zoom.
func quantileTargetsAbsolute(bucketMetric, groupBy, matcher, legend string) []target {
	return quantileTargetsAbsoluteIn(bucketMetric, groupBy, scope{matcher: matcher}, legend)
}

// quantileTargetsAbsoluteIn is quantileTargetsAbsolute for the series of a
// scope.
func quantileTargetsAbsoluteIn(bucketMetric, groupBy string, sc scope, legend string) []target {
	targets := make([]target, 0, len(quantiles))
	for _, p := range quantiles {
		targets = append(targets, query(
			fmt.Sprintf("histogram_quantile(%.2f, sum by (%s) (%s))", p.q, groupBy, sc.series(bucketMetric)),
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
func timeseriesPanel(title, unit string, targets ...target) *timeseriesPanelBuilder {
	vis := timeseries.NewVisualizationBuilder().
		Unit(unit).
		FillOpacity(10).
		GradientMode(common.GraphGradientModeOpacity).
		Legend(tableLegend())

	return &timeseriesPanelBuilder{panel: panel{title: title, targets: targets, viz: vis}, vis: vis}
}

// timelinePanel renders an integer PostgreSQL timeline ID over time as a
// stepped line, so an operator sees not only the current timeline but exactly
// when it changed (a promotion or failover), which a single current value (a
// stat or bar gauge) cannot convey. The unit is a plain count with no decimals.
func timelinePanel(title string, targets ...target) *timeseriesPanelBuilder {
	b := timeseriesPanel(title, units.Number, targets...)
	b.vis.
		FillOpacity(0).
		LineInterpolation(common.LineInterpolationStepAfter).
		LineWidth(2).
		Decimals(0)

	return b
}

// descriptionPanel builds a transparent markdown text panel that explains a
// row section (a Grafana row has no description field of its own).
func descriptionPanel(content string) *textPanelBuilder {
	return &textPanelBuilder{panel{
		transparent: true,
		viz:         text.NewVisualizationBuilder().Mode(text.TextModeMarkdown).Content(content),
	}}
}

// statPanel builds a stat panel showing the last value of its query targets,
// wired to the dashboard data source.
func statPanel(title, unit string, targets ...target) *statPanelBuilder {
	vis := stat.NewVisualizationBuilder().
		Unit(unit).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"})).
		GraphMode(common.BigValueGraphModeNone).
		// Color the value using Grafana's classic palette (one color per
		// series) rather than threshold coloring, so red stays reserved for
		// real alert/risk panels (none defined yet).
		ColorMode(common.BigValueColorModeValue).
		ColorScheme(dashboardv2beta1.NewFieldColorBuilder().Mode(dashboardv2beta1.FieldColorModeIdPaletteClassic)).
		// Cap the value/title font size: Grafana's auto-sizer otherwise blows
		// long strings (e.g. the "21 minutes" duration format) up until they
		// overflow and clip in the dense tiles.
		Text(common.NewVizTextDisplayOptionsBuilder().TitleSize(14).ValueSize(28))

	return &statPanelBuilder{panel: panel{title: title, targets: targets, viz: vis}, vis: vis}
}
