// Command grafana generates the Klio Grafana dashboard from code using the
// grafana-foundation-sdk. The dashboard is built against the Prometheus names
// of the OpenTelemetry metrics that Klio exports (see
// documentation/web/docs/user/opentelemetry.md) and is split into row
// sections: one for the client (plugin sidecar) metrics, one for the server
// metrics, and one for WAL replication lag (sourced from CloudNativePG's
// pg_stat_replication metrics).
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
	// dashboardUID is a stable identifier so re-imports update the same
	// dashboard instead of creating duplicates.
	dashboardUID = "klio"
	// dashboardTitle is the title shown in Grafana.
	dashboardTitle = "Klio"
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

// build assembles the full dashboard: the data source variable, then the
// client and server row sections with their panels.
func build() *dashboard.DashboardBuilder {
	builder := dashboard.NewDashboardBuilder(dashboardTitle).
		Uid(dashboardUID).
		Description("Backup and WAL observability for Klio, built from the Prometheus "+
			"export of Klio's OpenTelemetry metrics.").
		Tags([]string{"klio", "postgresql", "backup"}).
		Editable().
		Refresh("30s").
		Time("now-6h", "now").
		WithVariable(
			dashboard.NewDatasourceVariableBuilder(datasourceVar).
				Label("Data source").
				Type("prometheus").
				// Leave the current value unset: on load Grafana auto-selects the
				// default Prometheus data source and resolves $datasource to its
				// real UID.
				Current(dashboard.VariableOption{}),
		).
		WithVariable(
			labelVariable("namespace", "Namespace",
				`label_values({__name__=~"klio_.+"}, k8s_namespace_name)`),
		).
		WithVariable(
			labelVariable("server", "Server",
				`label_values(klio_server_uptime_seconds{k8s_namespace_name=~"$namespace"}, host_name)`),
		).
		WithVariable(
			labelVariable("cluster", "Cluster",
				`label_values(klio_server_wal_written_total{k8s_namespace_name=~"$namespace",host_name=~"$server"}, cluster_name)`),
		)

	y := 0
	layoutSection(builder, &y, "Client / Plugin", clientPanels())
	layoutSection(builder, &y, "Server", serverPanels())
	layoutSection(builder, &y, "WAL Replication Lag", replicationPanels())

	return builder
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

// layoutSection adds a row header at the current y offset, then packs the
// panels into a dense grid: each grid row is filled to the full 24-column width
// (slack is distributed across the row's panels) so there are no empty gaps
// between panels. y is advanced past the section.
func layoutSection(builder *dashboard.DashboardBuilder, y *int, title string, panels []sizedPanel) {
	builder.WithRow(dashboard.NewRowBuilder(title).
		GridPos(dashboard.GridPos{X: 0, Y: uint32(*y), W: 24, H: 1})) //nolint:gosec // small positive ints
	*y++

	for i := 0; i < len(panels); {
		// Greedily gather panels until the row would overflow 24 columns.
		rowW, j := 0, i
		for j < len(panels) && rowW+panels[j].w <= 24 {
			rowW += panels[j].w
			j++
		}
		if j == i { // a single panel wider than the grid: place it alone
			j, rowW = i+1, panels[i].w
		}
		row := panels[i:j]
		rowH, slack := 0, 24-rowW
		for _, p := range row {
			if p.h > rowH {
				rowH = p.h
			}
		}
		x := 0
		for k, p := range row {
			w := p.w + slack/len(row)
			if k < slack%len(row) {
				w++ // spread the remainder over the first panels
			}
			if k == len(row)-1 {
				w = 24 - x // last panel closes the row exactly
			}
			builder.WithPanel(p.place(dashboard.GridPos{
				X: uint32(x), Y: uint32(*y), W: uint32(w), H: uint32(rowH), //nolint:gosec // small positive ints
			}))
			x += w
		}
		*y += rowH
		i = j
	}
}

// labelVariable builds a multi-value Prometheus label_values template variable
// that defaults to "All" so the dashboard shows every series until filtered.
func labelVariable(name, label, query string) *dashboard.QueryVariableBuilder {
	return dashboard.NewQueryVariableBuilder(name).
		Label(label).
		Datasource(datasourceRef()).
		Query(dashboard.StringOrMap{String: cog.ToPtr(query)}).
		Refresh(dashboard.VariableRefreshOnDashboardLoad).
		Sort(dashboard.VariableSortAlphabeticalAsc).
		Multi(true).
		IncludeAll(true).
		AllValue(".+").
		Current(dashboard.VariableOption{
			Selected: cog.ToPtr(true),
			Text:     dashboard.StringOrArrayOfString{String: cog.ToPtr("All")},
			Value:    dashboard.StringOrArrayOfString{String: cog.ToPtr("$__all")},
		})
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
// (a narrow stat tile can hide all but the first).
func barGaugePanel(title, unit string, targets ...cog.Builder[variants.Dataquery]) *bargauge.PanelBuilder {
	panel := bargauge.NewPanelBuilder().
		Title(title).
		Datasource(datasourceRef()).
		Unit(unit).
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
