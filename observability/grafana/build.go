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
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

const (
	// dashboardUID is a stable identifier so re-imports update the same
	// dashboard instead of creating duplicates.
	dashboardUID = "klio"
	// dashboardTitle is the title shown in Grafana.
	dashboardTitle = "Klio"
)

// build assembles the full dashboard: the data source variable, then each
// row section with its panels.
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
