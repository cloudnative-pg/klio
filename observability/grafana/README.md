# Klio Grafana dashboard generator

This module generates the Klio Grafana dashboard
([`klio.json`](dashboards/klio.json)) from code, using the
[grafana-foundation-sdk](https://github.com/grafana/grafana-foundation-sdk).
It uses the dashboard schema v2 (Grafana 13.1 or later) and has two
sections, top-level rows that each have their own variables:

- **Clusters**, organized by PostgreSQL cluster: backups, WAL archiving, WAL
  replication lag and WAL restore.
- **Servers**, organized by Klio server: server health, retained backups,
  backup maintenance, served clusters, Kopia snapshots and WAL processing.

The dashboard visualizes the Prometheus export of Klio's OpenTelemetry
metrics. The panels are built in [`client.go`](client.go) (plugin sidecar
and WAL streaming client, `klio_plugin_*` and `klio_client_wal_*`),
[`server.go`](server.go) (Klio server, `klio_server_*`) and
[`replication.go`](replication.go) (CloudNativePG's
`cnpg_pg_stat_replication_*` metrics, which require CloudNativePG
monitoring scraped into the same Prometheus).

[`build.go`](build.go) assembles the dashboard: it declares a `datasource`
variable (so it is portable across Grafana installations), and each section
its own variables and rows. The panels are built with the SDK's schema v2
builders; [`convert.go`](convert.go) only adapts the visualization builders,
which still build the `v2beta1` visualization type.
[`main.go`](main.go) is just the CLI entrypoint (flag parsing, writing the
JSON) plus the shared panel and query helpers every row section builds on.

## Regenerating the dashboard

From the repository root:

```sh
task grafana:gen
```

This runs the generator in a container and writes
`observability/grafana/dashboards/klio.json` back into the repository. The
committed JSON must always match the generator output: `task
grafana:uncommitted` (part of `task grafana:ci`) regenerates it and fails if
it has drifted, so commit the regenerated file whenever you change the
generator or bump the SDK.

You can also run it directly:

```sh
go run . -output dashboards/klio.json
```

## Linting

`task grafana:ci` runs four checks:

- `grafana:go-test` — `go test` on the generator: catches PromQL mistakes gcx
  can't (a `histogram_quantile` query dropping the `le` label, a `rate()`
  call with a hardcoded window instead of `$__rate_interval`, a query
  referencing a variable that is not declared in its section), duplicate panel
  titles within a row, panels without a title or a description, and panel
  queries that leave out a variable of their section.
- `grafana:lint-builder` — `golangci-lint` on the generator Go code.
- `grafana:lint-dashboard` — lints the generated dashboard JSON with
  [`gcx`](https://github.com/grafana/gcx), which validates the PromQL of every
  panel target and checks panel units. The `uneditable-dashboard` rule is
  disabled because the dashboard ships editable on purpose, and the
  `panel-title-description` rule because the text panels that explain each
  section and row have no title (`go test` checks the other panels).
- `grafana:uncommitted` — fails if the committed JSON has drifted from the
  generator output.

## Using the dashboard

Import `dashboards/klio.json` into Grafana 13.1 or later and select your
Prometheus data source for the `datasource` variable. See
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

Open <http://localhost:3000/> and paste `dashboards/klio.json` via
**Dashboards → New → Import**, selecting your Prometheus data source. Or
load it automatically with Grafana's dashboard sidecar (`kubectl create`,
not `kubectl apply`, whose annotation would exceed the size limit):

```sh
kubectl create configmap klio-grafana-dashboard \
  --from-file=observability/grafana/dashboards/klio.json
kubectl label configmap klio-grafana-dashboard grafana_dashboard=1
```

The variables at the top of each section filter its panels.
