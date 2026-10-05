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

	"github.com/grafana/grafana-foundation-sdk/go/units"
)

// cnpgMatcher selects the CloudNativePG replication metrics for Klio's WAL
// streaming client. Unlike the klio_* metrics (which carry the OpenTelemetry
// k8s_namespace_name resource attribute), the cnpg_* metrics come from
// CloudNativePG's own exporter and use the `namespace` label; application_name
// is the replication application name Klio connects with.
const cnpgMatcher = `namespace=~"$namespace",application_name="klio"`

// replicationPanels returns the "WAL Replication Lag" section panels. They
// measure how far Klio's WAL streaming client trails the PostgreSQL primary,
// using CloudNativePG's pg_stat_replication metrics (exported to Prometheus as
// cnpg_pg_stat_replication_*). They require CloudNativePG monitoring to be
// scraped into the same Prometheus as Klio's metrics.
func replicationPanels() []sizedPanel {
	return []sizedPanel{
		sized(gridWidth, descriptionPanelHeight, descriptionPanel(
			"How far the WAL stored in tier 1 and tier 2 trails behind the PostgreSQL primary.")),
		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Tier-1 replication lag (bytes)", units.BytesIEC,
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_write_diff_bytes{%s}", cnpgMatcher),
				"{{pod}} write"),
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_flush_diff_bytes{%s}", cnpgMatcher),
				"{{pod}} flush"),
		).Description("Byte distance between the primary's current WAL LSN and the LSN Klio's streaming "+
			"client has written to disk and flushed.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Tier-1 replication lag (seconds)", units.Seconds,
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_write_lag_seconds{%s}", cnpgMatcher),
				"{{pod}} write"),
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_flush_lag_seconds{%s}", cnpgMatcher),
				"{{pod}} flush"),
		).Description("Time between a commit on the primary and Klio's streaming client writing to disk "+
			"and flushing the corresponding WAL.")),

		sized(largePanelWidth, mediumPanelHeight, timeseriesPanel("Tier-2 archival lag (bytes)", units.BytesIEC,
			query(
				fmt.Sprintf(
					"(max by (cluster_name) (klio_server_wal_latest_written_lsn_bytes{tier=\"tier1\",%s}) - "+
						"max by (cluster_name) (klio_server_wal_latest_written_lsn_bytes{tier=\"tier2\",%s})) ",
					walMatcher, walMatcher),
				"{{cluster_name}}",
			),
		).Description("Bytes difference between the WALs in tier 1 and tier 2 per cluster. ")),
	}
}
