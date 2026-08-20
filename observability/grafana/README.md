# Klio Grafana dashboard generator

This module generates the Klio Grafana dashboard
([`klio-dashboard.json`](klio-dashboard.json)) from code, using the
[grafana-foundation-sdk](https://github.com/grafana/grafana-foundation-sdk).

The dashboard visualizes the Prometheus export of Klio's OpenTelemetry
metrics. It is split into row sections:

- **Client / Plugin** — backup lifecycle metrics emitted by the plugin
  sidecar in each PostgreSQL pod (`klio_plugin_backup_*`), plus the streamed
  timeline and per-block send latency of the WAL streaming client it
  supervises as a child process (`klio_client_wal_*`). Built in
  [`client.go`](client.go).
- **Server** — WAL ingest, backup verification, base snapshot, the retention
  window of physical PostgreSQL backups (start/end time, LSN and timeline per
  cluster and tier), and queue metrics emitted by the Klio server StatefulSet
  (`klio_server_*`). Built in [`server.go`](server.go).
- **WAL Replication Lag** — how far Klio's WAL streaming client trails the
  PostgreSQL primary, from CloudNativePG's `cnpg_pg_stat_replication_*`
  metrics (requires CloudNativePG monitoring scraped into the same
  Prometheus). Built in [`replication.go`](replication.go).

[`build.go`](build.go) assembles the dashboard: it declares a `datasource`
template variable (so the dashboard is portable across Grafana
installations) and adds the row sections with their panels.
[`main.go`](main.go) is just the CLI entrypoint (flag parsing, writing the
JSON) plus the shared panel and query helpers every row section builds on.

## Regenerating the dashboard

From the repository root:

```sh
task grafana:gen
```

This runs the generator in a container and writes
`observability/grafana/klio-dashboard.json` back into the repository. The
committed JSON must always match the generator output: `task
grafana:uncommitted` (part of `task grafana:ci`) regenerates it and fails if
it has drifted, so commit the regenerated file whenever you change the
generator or bump the SDK.

You can also run it directly:

```sh
go run . -output klio-dashboard.json
```

## Linting

`task grafana:ci` runs four checks:

- `grafana:go-test` — `go test` on the generator: catches PromQL mistakes gcx
  can't (a `histogram_quantile` query dropping the `le` label, a `rate()`
  call with a hardcoded window instead of `$__rate_interval`, a query
  referencing an undeclared dashboard variable) and duplicate panel titles
  within a row.
- `grafana:lint-builder` — `golangci-lint` on the generator Go code.
- `grafana:lint-dashboard` — lints the generated dashboard JSON with
  [`gcx`](https://github.com/grafana/gcx), which validates the PromQL of every
  panel target and checks panel units. The `uneditable-dashboard` rule is
  disabled because the dashboard ships editable on purpose.
- `grafana:uncommitted` — fails if the committed JSON has drifted from the
  generator output.

## Using the dashboard

Import `klio-dashboard.json` into Grafana and select your Prometheus data
source for the `datasource` variable. See
[the documentation](../../documentation/web/docs/user/grafana-dashboards.md)
for the Prometheus prerequisites.

## Previewing on a cluster

This mirrors the
[CloudNativePG quickstart](https://cloudnative-pg.io/docs/current/quickstart/#grafana-dashboard).
Deploy Prometheus + Grafana with the
[kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
chart (using the CloudNativePG sample values):

```sh
helm repo add prometheus-community \
  https://prometheus-community.github.io/helm-charts
helm upgrade --install \
  -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/main/docs/src/samples/monitoring/kube-stack-config.yaml \
  prometheus-community prometheus-community/kube-prometheus-stack
```

Make Prometheus scrape Klio's metrics by deploying a `ServiceMonitor` — or a
`PodMonitor` if the collector's `Service` has no labels — for the OTel
collector (see
[`operator/config/samples/opentelemetry/base/otel_collector_svc_monitor.yaml`](../../operator/config/samples/opentelemetry/base/otel_collector_svc_monitor.yaml)
and the [OpenTelemetry guide](../../documentation/web/docs/user/opentelemetry.md)).

Port-forward Grafana (log in with `admin` / `prom-operator`):

```sh
kubectl port-forward svc/prometheus-community-grafana 3000:80
```

Open <http://localhost:3000/> and import `klio-dashboard.json` via
**Dashboards → New → Import**, selecting your Prometheus data source. Or load
it automatically with Grafana's dashboard sidecar:

```sh
kubectl create configmap klio-grafana-dashboard \
  --from-file=klio-dashboard.json=observability/grafana/klio-dashboard.json
kubectl label configmap klio-grafana-dashboard grafana_dashboard=1
```

The `namespace` and `cluster` template variables at the top of the dashboard
filter the panels.
