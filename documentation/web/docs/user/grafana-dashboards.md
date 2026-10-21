---
sidebar_position: 11
---

# Grafana Dashboards

Klio ships two Grafana dashboard templates that visualize the metrics it
exposes. The dashboards are generated from code with the
[Grafana Foundation SDK](https://github.com/grafana/grafana-foundation-sdk)
and committed to the repository under `observability/grafana/dashboards/`:

- **Klio Clusters** (`klio-clusters.json`), organized around the PostgreSQL
  clusters that Klio backs up.
- **Klio Servers** (`klio-servers.json`), organized around the Klio servers.

Each dashboard has its own filters, and every filter applies to all of its
panels. A link at the top of each dashboard opens the other one.

The dashboards query the Prometheus export of Klio's OpenTelemetry metrics,
so they complement the [OpenTelemetry](opentelemetry.md) setup rather than
replacing it.

## What the dashboards show

The **Klio Clusters** dashboard is split into row sections:

- **Backups** — the backup lifecycle as seen by the plugin sidecar
  running in each PostgreSQL pod: backups in progress, time since the last
  backup started, succeeded and failed, the latest backup duration, the
  p50/p90/p99 backup duration distribution, backup run and failure rates,
  and the backup success ratio.

- **Retained backups** — the retention window of the physical PostgreSQL
  backups the Klio server keeps for each cluster: counts by tier,
  latest/oldest backup age, start/end LSN and PostgreSQL timeline.
- **Backup maintenance** — tier-2 relay, backup verification and
  maintenance run totals per cluster.
- **WAL archiving** — the WAL streaming client the sidecar supervises as a
  child process (the PostgreSQL timeline it is currently streaming and the
  p50/p90/p99 latency of sending a WAL block to the server), and the WAL the
  server writes: throughput and freshness per tier, the latest written LSN
  and the p50/p90/p99 tier-2 upload duration.

![Klio Clusters dashboard](images/clusters_dashboard.png)
- **WAL replication lag** — how far behind Klio is in copying WAL:
  - **Tier-1 replication lag (bytes)** and **(seconds)**: how much WAL,
    and how much time, Klio is behind the PostgreSQL primary. The *write*
    line is WAL Klio has received; the *flush* line is WAL Klio has
    safely saved to disk. These two panels need CloudNativePG monitoring
    (see the prerequisites below).
  - **Tier-2 archival lag (bytes)**: how much WAL is on Klio's local disk
    (tier 1) but not yet copied to remote storage (tier 2).

![Klio WAL replication lag metrics](images/klio_wal_replication_lag_metrics.png)

- **WAL restore** — the WAL restores the plugin serves back to PostgreSQL,
  per instance (pod), since each instance restores its own WAL through its
  own prefetch spool: the p50/p90/p99 restore duration (how long PostgreSQL
  waited for each WAL file, not the replay that follows), as both a
  rolling-window and a since-restart total variant, the restore rate and
  total by tier and outcome, the prefetch hit ratio, and the share of time
  PostgreSQL spent waiting on the plugin (near 0% when recovery is limited
  by replay, not by Klio).

The **Klio Servers** dashboard is split into row sections:

- **Server health** — the uptime of each Klio server and its embedded NATS
  JetStream queue.
- **Served clusters** — retained backups, latest backup age, time since the
  last WAL written and WAL throughput of the clusters backed by the selected
  servers.
- **Kopia snapshots** — the base snapshot inventory, including file and
  directory counts.
- **WAL processing** — p50/p90/p99 WAL block and get duration by path, stage
  and tier, each as both a rolling-window and a since-restart total variant.

![Klio Servers dashboard](images/servers_dashboard.png)

Some panels need extra context to interpret correctly. Two are derived
from the alerting guidance in [OpenTelemetry](opentelemetry.md):

- **Time since last WAL written** surfaces the staleness signal
  described under *Alerting on stalled WAL processing*: a stale tier-1 value
  means PostgreSQL is no longer shipping WALs, while a stale tier-2 value
  means the remote backend is no longer receiving them.
- **Tier-2 archival lag (bytes)** plots the LSN difference between
  tier 1 (local disk) and tier 2 (remote storage). Read together with the
  staleness panel, it tells a slow pipeline (timestamps advancing, gap
  growing) apart from a stalled one (timestamps and LSN both frozen).

One more is a statistical caveat rather than an alerting signal:

- **WAL restore duration percentiles** pool prefetch hits, a local rename,
  and misses, a wait on a download, which are orders of magnitude apart. With
  a high hit ratio the lower percentiles describe hits, around a
  millisecond, and misses only show in the upper percentiles. Read them
  together with the **WAL restore prefetch hit ratio**, whose fall over time
  means prefetch is no longer keeping up with replay on that instance, and
  the **WAL restore wait share**, which shows how much the waiting costs.

## Prerequisites

The dashboards read the Prometheus names of Klio's metrics (for example
`klio_plugin_backup_runs_total` and `klio_server_wal_written_total`). You
therefore need Prometheus scraping those metrics. Any of the export paths
described in [OpenTelemetry](opentelemetry.md) works:

- An OpenTelemetry Collector with a Prometheus exporter that Prometheus
  scrapes.
- The Klio Prometheus exporter (`OTEL_METRICS_EXPORTER=prometheus`),
  scraped directly.

The **WAL replication lag** row of the Klio Clusters dashboard additionally
reads CloudNativePG's `cnpg_pg_stat_replication_*` metrics. To populate it,
scrape the CloudNativePG cluster monitoring (its `PodMonitor`) into the same
Prometheus. The rest of the dashboards work without it.

The LSN table panels (**Latest backup LSN**, **Oldest backup LSN**,
**Latest written LSN**) need Grafana 13 or later: they use SQL
Expressions, available starting in that version. Without it, these
three panels show an error instead of a table.

:::note
When you route metrics through an OpenTelemetry Collector, enable
`resource_to_telemetry_conversion` on the Prometheus exporter so that
resource attributes such as the pod and namespace become Prometheus labels.
The sample collector under
`operator/config/samples/opentelemetry/base/otel_collector.yaml` already does
this.
:::

## Importing the dashboards

1. In Grafana, go to **Dashboards → New → Import**.
1. Upload `observability/grafana/dashboards/klio-clusters.json` or paste its
   contents.
1. When prompted, select your Prometheus data source for the `datasource`
   variable.
1. Repeat for `observability/grafana/dashboards/klio-servers.json`.

Each dashboard declares a `datasource` template variable, so it is portable
across Grafana installations and is not tied to a specific data source UID.
The other template variables at the top filter the panels.

On the Klio Clusters dashboard:

- `namespace`: the namespace where the PostgreSQL cluster runs, matched
  against `k8s.namespace.name`.
- `cluster`: the PostgreSQL cluster, matched against `cluster_name`. It
  lists every cluster that emits Klio plugin metrics, including clusters
  that only restore from a Klio server.
- `pod` (*Instance*): the PostgreSQL pod, matched against `k8s.pod.name`.
  It scopes the panels built from the plugin metrics.

The server-side metrics carry the server's own namespace, not the cluster's,
so the per-cluster server panels are filtered by `cluster` only.

On the Klio Servers dashboard:

- `namespace` (*Server namespace*): the namespace where the Klio server
  runs, which can differ from the namespace of the clusters it backs up.
- `server`: the Klio server, matched against the OpenTelemetry
  `service.name` (not the pod host name, which two servers of the same name
  in different namespaces would share).
- `cluster` (*Served cluster*): the clusters backed up by the selected
  servers, from any namespace.

Every aggregation groups by the identifying label (cluster, server, tier),
so multiple clusters or servers are never folded into a single misleading
value.

:::note
Give each Klio server a distinct `OTEL_SERVICE_NAME` (as the sample server
manifests do). The `server` variable of the Klio Servers dashboard
identifies servers by their OpenTelemetry `service.name`; if it is left
unset, every server reports the SDK default (`unknown_service:klio`) and
they collapse into a single, indistinguishable entry.
:::

### Example: kube-prometheus-stack

This example follows the same flow as the
[CloudNativePG quickstart](https://cloudnative-pg.io/docs/current/quickstart/#grafana-dashboard),
using the
[kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
chart. Adapt it to your own Prometheus/Grafana if you run a different setup —
only the metric prerequisites described above are required.

Install Prometheus and Grafana:

```sh
helm repo add prometheus-community \
  https://prometheus-community.github.io/helm-charts
helm upgrade --install \
  -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/main/docs/src/samples/monitoring/kube-stack-config.yaml \
  prometheus-community prometheus-community/kube-prometheus-stack
```

Ensure Prometheus scrapes Klio's metrics by deploying a `ServiceMonitor` (or a
`PodMonitor`, if the collector's `Service` has no labels) for the
OpenTelemetry collector's Prometheus exporter — see
`operator/config/samples/opentelemetry/base/otel_collector_svc_monitor.yaml`.

Port-forward Grafana and log in with `admin` / `prom-operator`:

```sh
kubectl port-forward svc/prometheus-community-grafana 3000:80
```

Open `http://localhost:3000/` and import the two JSON files under
`observability/grafana/dashboards/` via **Dashboards → New → Import**,
selecting your Prometheus data source.

Alternatively, load them automatically through the Grafana dashboard sidecar
with a labeled `ConfigMap`:

```sh
kubectl create configmap klio-grafana-dashboards \
  --from-file=observability/grafana/dashboards/
kubectl label configmap klio-grafana-dashboards grafana_dashboard=1
```

## Regenerating the dashboards

The committed JSON is generated from the Go program under
`observability/grafana/`. To regenerate it after changing the generator (or
after a `grafana-foundation-sdk` bump), run from the repository root:

```sh
task grafana:gen
```

CI runs `task grafana:uncommitted`, which regenerates the dashboards and
fails if the committed JSON has drifted from the generator output. Commit
the regenerated file whenever it changes.
