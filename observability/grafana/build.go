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

// section is a dashboard row.
type section struct {
	title       string
	description string
	panels      []sizedPanel
}

// dashboards returns every Klio dashboard.
func dashboards() []*dashboard.DashboardBuilder {
	return []*dashboard.DashboardBuilder{clusterDashboard(), serverDashboard()}
}

// clusterDashboard builds the dashboard organized by PostgreSQL cluster.
func clusterDashboard() *dashboard.DashboardBuilder {
	// Sourced from the plugin metrics, so restore-only clusters are listed.
	builder := newDashboard("klio-clusters", "Klio Clusters",
		"Backups, WAL archiving and WAL restore of the PostgreSQL clusters backed up by Klio.").
		WithVariable(labelVariable("namespace", "Namespace",
			`label_values({__name__=~"klio_(plugin|client)_.+"}, k8s_namespace_name)`)).
		WithVariable(labelVariable("cluster", "Cluster",
			`label_values({__name__=~"klio_(plugin|client)_.+",k8s_namespace_name=~"$namespace"}, cluster_name)`)).
		WithVariable(labelVariable("pod", "Instance",
			`label_values({__name__=~"klio_(plugin|client)_.+",k8s_namespace_name=~"$namespace",`+
				`cluster_name=~"$cluster"}, k8s_pod_name)`))

	layoutDashboard(builder,
		"*Namespace* is where the PostgreSQL cluster runs. *Cluster* lists every cluster that emits Klio "+
			"plugin metrics, restore-only clusters included. *Instance* narrows the plugin panels to "+
			"individual PostgreSQL pods.",
		section{
			"Backups",
			"Base backup lifecycle as seen by the plugin sidecar running in each PostgreSQL pod.",
			backupPanels(),
		},
		section{
			"Retained backups",
			"Retention window of the physical PostgreSQL backups the Klio server keeps for each cluster.",
			retainedBackupPanels(),
		},
		section{
			"Backup maintenance",
			"Asynchronous operations the Klio server runs on the backups: tier-2 relay, verification and " +
				"maintenance.",
			backupMaintenancePanels(),
		},
		section{
			"WAL archiving",
			"WAL streamed by the client in the PostgreSQL pod, and written by the Klio server to each tier.",
			append(walStreamingPanels(), walIngestPanels()...),
		},
		section{
			"WAL replication lag",
			"How far the WAL stored in tier 1 and tier 2 trails behind the PostgreSQL primary.",
			replicationPanels(),
		},
		section{
			"WAL restore",
			"WAL files the plugin serves to PostgreSQL during recovery or on a replica, per instance.",
			walRestorePanels(),
		},
	)

	return builder
}

// serverDashboard builds the dashboard organized by Klio server.
func serverDashboard() *dashboard.DashboardBuilder {
	// Servers are identified by service.name, which stays unique when two
	// servers share a host_name. Served clusters are narrowed by server, not
	// namespace, to keep clusters served cross-namespace.
	builder := newDashboard("klio-servers", "Klio Servers",
		"Health, served clusters, Kopia snapshots and WAL processing of the Klio servers.").
		WithVariable(labelVariable("namespace", "Server namespace",
			`label_values(klio_server_uptime_seconds, k8s_namespace_name)`)).
		WithVariable(labelVariable("server", "Server",
			`label_values(klio_server_uptime_seconds{k8s_namespace_name=~"$namespace"}, service_name)`)).
		WithVariable(labelVariable("cluster", "Served cluster",
			`label_values(klio_server_wal_written_total{service_name=~"$server"}, cluster_name)`))

	layoutDashboard(builder,
		"*Server namespace* is where the Klio server runs, not where the clusters it backs run. *Server* "+
			"selects one or more Klio servers. *Served cluster* lists the clusters backed by the selected "+
			"servers, from any namespace.",
		section{
			"Server health",
			"Uptime of each Klio server and the messages waiting in its embedded queue.",
			serverHealthPanels(),
		},
		section{
			"Served clusters",
			"Backups and WAL of the clusters backed by the selected servers.",
			servedClusterPanels(),
		},
		section{
			"Kopia snapshots",
			"Base backup snapshots stored in the Kopia repository of each tier.",
			snapshotPanels(),
		},
		section{
			"WAL processing",
			"Latency of the WAL operations on the server, aggregated across the selected served clusters.",
			walProcessingPanels(),
		},
	)

	return builder
}

// newDashboard returns a dashboard with the settings shared by every Klio
// dashboard.
func newDashboard(uid, title, description string) *dashboard.DashboardBuilder {
	return dashboard.NewDashboardBuilder(title).
		Uid(uid).
		Description(description+" Built from the Prometheus export of Klio's OpenTelemetry metrics.").
		Tags([]string{"klio", "postgresql", "backup"}).
		Editable().
		Refresh("30s").
		Time("now-6h", "now").
		// Variables are not carried over: their meaning differs per dashboard.
		Link(dashboard.NewDashboardLinkBuilder("Klio dashboards").
			Type(dashboard.DashboardLinkTypeDashboards).
			Tags([]string{"klio"}).
			AsDropdown(false).
			KeepTime(true).
			IncludeVars(false).
			TargetBlank(false)).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder(datasourceVar).
				Label("Data source").
				Type("prometheus").
				// Leave the current value unset: on load Grafana auto-selects the
				// default Prometheus data source and resolves $datasource to its
				// real UID.
				Current(dashboard.VariableOption{}),
		)
}

// layoutDashboard adds the filter note, then each section.
func layoutDashboard(builder *dashboard.DashboardBuilder, note string, sections ...section) {
	builder.WithPanel(descriptionPanel(note).
		GridPos(dashboard.GridPos{X: 0, Y: 0, W: gridWidth, H: descriptionPanelHeight}))
	y := descriptionPanelHeight
	for _, s := range sections {
		panels := append([]sizedPanel{sized(gridWidth, descriptionPanelHeight, descriptionPanel(s.description))},
			s.panels...)
		layoutSection(builder, &y, s.title, panels)
	}
}

// layoutSection adds a row header at the current y offset, then packs the
// panels into a dense grid: each grid row is filled to the full 24-column
// width (slack is distributed across the row's panels) so there are no empty
// gaps between panels. Widths that add up to 24 are used as they are. y is
// advanced past the section.
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
