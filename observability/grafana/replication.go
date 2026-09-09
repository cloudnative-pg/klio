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
		// WAL lag as the byte distance between the primary's current LSN and
		// the LSN Klio's streaming client has flushed. Reported in bytes;
		// Grafana scales the unit for display.
		sized(8, panelHeight, timeseriesPanel("Tier-1 Replication lag (Bytes)", "bytes",
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_flush_diff_bytes{%s}", cnpgMatcher),
				"{{pod}}"),
		).Description("Byte distance between the primary's current WAL LSN and the LSN Klio's streaming "+
			"client has flushed, from CloudNativePG's pg_stat_replication.")),
		// Flush lag in seconds: time between a commit on the primary and Klio's
		// client flushing the corresponding WAL.
		sized(8, panelHeight, timeseriesPanel("Tier-1 Replication lag (Seconds)", "s",
			query(
				fmt.Sprintf("cnpg_pg_stat_replication_flush_lag_seconds{%s}", cnpgMatcher),
				"{{pod}}"),
		).Description("Time between a commit on the primary and Klio's streaming client flushing the "+
			"corresponding WAL, from CloudNativePG's pg_stat_replication.")),
		// Derived: tier-2 archival backlog as the LSN gap between what the
		// server has on local disk (tier1) and what has been archived remotely
		// (tier2), in MiB. Each side is reduced with max by (cluster_name) first
		// so the subtraction is a one-to-one vector match on cluster_name even
		// when several servers export the same cluster's WAL series.
		sized(8, panelHeight, timeseriesPanel("Tier-2 archival lag (Tier-1 LSN gap)", "mbytes",
			query(
				fmt.Sprintf(
					"(max by (cluster_name) (klio_server_wal_latest_written_lsn_bytes{tier=\"tier1\",%s}) - "+
						"max by (cluster_name) (klio_server_wal_latest_written_lsn_bytes{tier=\"tier2\",%s})) "+
						"/ 1024 / 1024",
					walMatcher, walMatcher),
				"{{cluster_name}}",
			),
		).Description("LSN distance between tier 1 (local disk) and tier 2 (remote storage) per cluster. "+
			"A growing gap means remote archival is falling behind. Scoped by $server and $cluster (not "+
			"$namespace), since the server tags these series with its own namespace.")),
	}
}
