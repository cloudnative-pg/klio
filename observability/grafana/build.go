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

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/resource"
)

const (
	// dashboardName is the stable name (and UID) of the dashboard resource, so
	// re-imports update the same dashboard instead of creating duplicates.
	dashboardName = "klio"
	// dashboardTitle is the title shown in Grafana.
	dashboardTitle = "Klio"
)

// section is a dashboard row.
type section struct {
	title       string
	description string
	panels      []sizedPanel
}

// group is a top-level row of the dashboard: its sections, with the
// variables that filter their panels.
type group struct {
	title     string
	note      string
	variables []*dashboardv2.QueryVariableBuilder
	sections  []section
}

// build assembles the dashboard as a Kubernetes-style resource: one
// top-level row per entity, each with its own section variables.
func build() *resource.ManifestBuilder {
	builder := dashboardv2.NewDashboardBuilder(dashboardTitle).
		Description("Backups, WAL archiving and WAL restore of the PostgreSQL clusters backed up by Klio, and " +
			"the Klio servers that back them up. Built from the Prometheus export of Klio's OpenTelemetry " +
			"metrics.").
		Tags([]string{"klio", "postgresql", "backup"}).
		Editable(true).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().
			From("now-6h").
			To("now").
			AutoRefresh("30s").
			Timezone("browser")).
		// Only the data source is dashboard-wide: every other filter is a
		// variable of its group.
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder(datasourceVar).
			Label("Data source").
			PluginId("prometheus").
			Hide(dashboardv2.VariableHideDontHide).
			Refresh(dashboardv2.VariableRefreshOnDashboardLoad).
			// Leave the current value unset: on load Grafana auto-selects the
			// default Prometheus data source.
			Current(dashboardv2.VariableOption{
				Text:  dashboardv2.StringOrArrayOfString{String: cog.ToPtr("")},
				Value: dashboardv2.StringOrArrayOfString{String: cog.ToPtr("")},
			}).
			Options([]dashboardv2.VariableOption{}))

	layout := dashboardv2.NewRowsBuilder()
	ids := 0
	for _, t := range []group{clustersGroup(), serversGroup()} {
		built, err := t.build(builder, &ids)
		if err != nil {
			builder.RecordError(t.title, err)
			continue
		}
		layout.Row(built)
	}
	builder.RowsLayout(layout)

	return dashboardv2.Manifest(dashboardName, builder)
}

// clustersGroup is the group organized by PostgreSQL cluster.
func clustersGroup() group {
	return group{
		title: "Clusters",
		note: "*Namespace* is where the PostgreSQL cluster runs. *Cluster* lists every cluster that emits Klio " +
			"plugin metrics, restore-only clusters included. *Instance* narrows the plugin panels to " +
			"individual PostgreSQL pods.",
		variables: []*dashboardv2.QueryVariableBuilder{
			// Sourced from the plugin metrics, so restore-only clusters are listed.
			labelVariable("namespace", "Namespace",
				`label_values({__name__=~"klio_(plugin|client)_.+"}, k8s_namespace_name)`).
				AllValue(matchAny),
			// "All" expands to the clusters of the selected namespaces. Matching
			// any value would also select clusters from other namespaces on the
			// panels that are not filtered by namespace. Refreshed with the time
			// range, so "All" picks up clusters that appear after the dashboard
			// loads.
			labelVariable("cluster", "Cluster",
				`label_values({__name__=~"klio_(plugin|client)_.+",k8s_namespace_name=~"$namespace"}, cluster_name)`).
				Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged),
			labelVariable("pod", "Instance",
				`label_values({__name__=~"klio_(plugin|client)_.+",k8s_namespace_name=~"$namespace",`+
					`cluster_name=~"$cluster"}, k8s_pod_name)`).
				AllValue(matchAny),
		},
		sections: []section{
			{
				"Backups",
				"Base backup lifecycle as seen by the plugin sidecar running in each PostgreSQL pod.",
				backupPanels(),
			},
			{
				"Retained backups",
				"Retention window of the physical PostgreSQL backups the Klio server keeps for each cluster.",
				retainedBackupPanels(),
			},
			{
				"Backup maintenance",
				"Asynchronous operations the Klio server runs on the backups: tier-2 relay, verification and " +
					"maintenance.",
				backupMaintenancePanels(),
			},
			{
				"WAL archiving",
				"WAL streamed by the client in the PostgreSQL pod, and written by the Klio server to each tier.",
				append(walStreamingPanels(), walIngestPanels()...),
			},
			{
				"WAL replication lag",
				"How far the WAL stored in tier 1 and tier 2 trails behind the PostgreSQL primary.",
				replicationPanels(),
			},
			{
				"WAL restore",
				"WAL files the plugin serves to PostgreSQL during recovery or on a replica, per instance.",
				walRestorePanels(),
			},
		},
	}
}

// serversGroup is the group organized by Klio server. Its variables are
// named apart from the ones of the Clusters group, so the URL keeps both
// selections.
func serversGroup() group {
	return group{
		title: "Servers",
		note: "*Server namespace* is where the Klio server runs, not where the clusters it backs run. *Server* " +
			"selects one or more Klio servers. *Served cluster* lists the clusters backed by the selected " +
			"servers, from any namespace.",
		variables: []*dashboardv2.QueryVariableBuilder{
			labelVariable("server_namespace", "Server namespace",
				`label_values(klio_server_uptime_seconds, k8s_namespace_name)`).
				AllValue(matchAny),
			// The Server name, taken from its pod name `<server>-klio-<ordinal>`.
			// Together with the namespace it identifies a server. "All" expands
			// to the servers of the selected namespaces, and is refreshed, as the
			// cluster variable of the Clusters group.
			labelVariable("server", "Server",
				`label_values(klio_server_uptime_seconds{k8s_namespace_name=~"$server_namespace"}, k8s_pod_name)`).
				Regex(`/^(.+)-klio-[0-9]+$/`).
				Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged),
			// Narrowed by server, not namespace, to keep clusters served
			// cross-namespace. Not sourced from klio_server_wal_written_total: a
			// server that receives no WAL, such as a read-only one, would list
			// no clusters. Its "All" also matches the series without
			// cluster_name, so it hides no server (see servedClusterJoin).
			labelVariable("served_cluster", "Served cluster",
				`label_values({__name__=~"klio_server_.+",`+serverMatcher+`}, cluster_name)`).
				AllValue(".*"),
		},
		sections: []section{
			{
				"Server health",
				"Uptime of each Klio server and the messages waiting in its embedded queue.",
				serverHealthPanels(),
			},
			{
				"Served clusters",
				"Backups and WAL of the clusters backed by the selected servers.",
				servedClusterPanels(),
			},
			{
				"Kopia snapshots",
				"Base backup snapshots stored in the Kopia repository of each tier.",
				snapshotPanels(),
			},
			{
				"WAL processing",
				"Latency of the WAL operations on the server, aggregated across the selected served clusters.",
				walProcessingPanels(),
			},
		},
	}
}

// build adds the panels of the group to the dashboard elements and returns
// its row: a header-less row with the filter note, then one row per section.
func (t group) build(builder *dashboardv2.DashboardBuilder, ids *int) (*dashboardv2.RowBuilder, error) {
	variables := make([]dashboardv2.VariableKind, 0, len(t.variables))
	for _, v := range t.variables {
		built, err := v.Build()
		if err != nil {
			return nil, err
		}
		variables = append(variables, dashboardv2.VariableKind{QueryVariableKind: &built})
	}

	note, err := gridOf(builder, ids, []sizedPanel{sized(gridWidth, descriptionPanelHeight, descriptionPanel(t.note))})
	if err != nil {
		return nil, err
	}
	rows := dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("").HideHeader(true).GridLayout(note))
	for _, s := range t.sections {
		panels := append([]sizedPanel{sized(gridWidth, descriptionPanelHeight, descriptionPanel(s.description))},
			s.panels...)
		grid, err := gridOf(builder, ids, panels)
		if err != nil {
			return nil, fmt.Errorf("section %q: %w", s.title, err)
		}
		rows.Row(dashboardv2.NewRowBuilder().Title(s.title).GridLayout(grid))
	}

	return dashboardv2.NewRowBuilder().
		Title(t.title).
		Variables(variables).
		RowsLayout(rows), nil
}

// gridOf packs the panels into a dense grid, adds each one to the dashboard
// elements and returns the grid that references them.
func gridOf(builder *dashboardv2.DashboardBuilder, ids *int, panels []sizedPanel) (*dashboardv2.GridBuilder, error) {
	grid := dashboardv2.NewGridBuilder()
	for _, p := range pack(panels) {
		v1, err := p.panel.place(dashboard.GridPos{
			X: uint32(p.pos.X), Y: uint32(p.pos.Y), W: uint32(p.pos.W), H: uint32(p.pos.H), //nolint:gosec // small positive ints
		}).Build()
		if err != nil {
			return nil, err
		}
		*ids++
		panel, err := convertPanel(v1, *ids)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf("panel-%d", *ids)
		builder.Panel(key, built[dashboardv2.PanelKind]{panel})
		grid.Item(dashboardv2.NewGridItemBuilder().
			Name(key).
			X(int64(p.pos.X)).
			Y(int64(p.pos.Y)).
			Width(int64(p.pos.W)).
			Height(int64(p.pos.H)))
	}

	return grid, nil
}

// placedPanel is a panel with its position in the grid of its row.
type placedPanel struct {
	panel sizedPanel
	pos   gridPos
}

// gridPos is a position in a 24-column grid.
type gridPos struct {
	X, Y, W, H int
}

// pack lays the panels out in a dense grid: each grid row is filled to the
// full 24-column width (slack is distributed across the row's panels) so
// there are no empty gaps between panels. Widths that add up to 24 are used
// as they are.
func pack(panels []sizedPanel) []placedPanel {
	placed := make([]placedPanel, 0, len(panels))
	y := 0
	for i := 0; i < len(panels); {
		// Greedily gather panels until the row would overflow 24 columns.
		rowW, j := 0, i
		for j < len(panels) && rowW+panels[j].w <= gridWidth {
			rowW += panels[j].w
			j++
		}
		if j == i { // a single panel wider than the grid: place it alone
			j, rowW = i+1, panels[i].w
		}
		row := panels[i:j]
		rowH, slack := 0, gridWidth-rowW
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
				w = gridWidth - x // last panel closes the row exactly
			}
			placed = append(placed, placedPanel{panel: p, pos: gridPos{X: x, Y: y, W: w, H: rowH}})
			x += w
		}
		y += rowH
		i = j
	}

	return placed
}

// matchAny is the "All" value of the variables whose "All" must match any
// series, whatever the values listed by the variable.
const matchAny = ".+"

// labelVariable builds a multi-value Prometheus label_values variable that
// defaults to "All" so the dashboard shows every series until filtered.
// Unless the caller sets AllValue, "All" expands to the values the variable
// lists.
func labelVariable(name, label, query string) *dashboardv2.QueryVariableBuilder {
	return dashboardv2.NewQueryVariableBuilder(name).
		Label(label).
		Hide(dashboardv2.VariableHideDontHide).
		Query(built[dashboardv2.DataQueryKind]{dashboardv2.DataQueryKind{
			Kind:       "DataQuery",
			Group:      "prometheus",
			Version:    "v0",
			Datasource: &dashboardv2.Dashboardv2DataQueryKindDatasource{Name: cog.ToPtr(datasourceRefUID())},
			Spec: map[string]any{
				"qryType": 1,
				"query":   query,
				"refId":   "PrometheusVariableQueryEditor-VariableQuery",
			},
		}}).
		Definition(query).
		Refresh(dashboardv2.VariableRefreshOnDashboardLoad).
		Sort(dashboardv2.VariableSortAlphabeticalAsc).
		Options([]dashboardv2.VariableOption{}).
		Multi(true).
		IncludeAll(true).
		AllowCustomValue(true).
		Current(dashboardv2.VariableOption{
			Text:  dashboardv2.StringOrArrayOfString{String: cog.ToPtr("All")},
			Value: dashboardv2.StringOrArrayOfString{String: cog.ToPtr("$__all")},
		})
}

// built adapts an already built value to the SDK builder interface.
type built[T any] struct {
	value T
}

// Build returns the value.
//
//nolint:ireturn // the SDK builder interface returns the built type
func (b built[T]) Build() (T, error) {
	return b.value, nil
}
