# README

> **Warning**
> This directory is a **test environment for validating the Klio Grafana
> dashboard**, not a reference architecture. Do not copy its patterns into a
> production deployment. In particular:
> - It reflects the private-key half of TLS secrets into namespaces that
>   never use them (see the Topology section below), purely to keep the
>   sample declarative. A real deployment should never let a private key
>   leave the namespace that needs it.
> - It deliberately reuses/overloads names across namespaces (e.g. two
>   independent Klio servers both named `klio-a`) to exercise a dashboard
>   disambiguation edge case. Overloading names like this is a testing
>   device here, not something to replicate on purpose.

This directory contains two sample environments exercising Klio's
OpenTelemetry integration, both sharing the OTel collector / Jaeger /
Prometheus stack defined in `base/`. Every CNPG cluster also gets its own
`PodMonitor` (see `single/cluster/cluster_pod_monitor.yaml`), so Prometheus
scrapes CloudNativePG's own `cnpg_pg_stat_replication_*` metrics in
addition to what the OTel collector exports — the Grafana dashboard's WAL
Replication Lag row needs this (see
`documentation/web/docs/user/grafana-dashboards.md`):

- [`single/`](single): one Klio server and one CNPG cluster. Start here if
  you just want to see OpenTelemetry wired up.
- [`multi/`](multi): two Klio servers and four CNPG clusters distributed
  across them, one of which lives in a different namespace than the
  servers. It reuses `single/`'s Server and Cluster/PluginConfiguration
  definitions (see `multi/team-a`) rather than duplicating them, and exists
  to validate the Grafana dashboard's `$namespace`, `$server` and `$cluster`
  template variables and its per-tier/per-cluster aggregations against more
  than one server, cluster or namespace — something `single/` can't
  exercise.

## Topology

`multi/`'s four clusters:

| Cluster     | Namespace | Backed by |
|-------------|-----------|-----------|
| cluster-a   | default   | klio-a    |
| cluster-b   | default   | klio-b    |
| cluster-c   | team-c    | klio-b    |
| cluster-d   | team-d    | klio-a (independent server also named "klio-a") |

`klio-b` intentionally backs clusters in two different namespaces, since a
single shared backup server serving multiple application namespaces is a
realistic multi-tenant deployment and the case most likely to expose
dashboard attribution bugs. `cluster-d` is backed by a *second*, independent
Klio server that happens to also be named "klio-a" (see `multi/team-d`):
since a Server's StatefulSet pod name is derived from the Server's own name
alone, both servers' pods are named "klio-a-klio-0", giving them an
identical host_name label — a case the dashboard's `$server` variable
cannot disambiguate on its own.

Every Klio server has its own certificate authority, signing only that
server's own **client** certificates — no CA is ever shared between
servers for that purpose. `cluster-c`'s client certificate is therefore
requested next to `klio-b`'s CA/`Issuer` in `default` and mirrored into
`team-c` by kubernetes-reflector; `cluster-a`, `cluster-b` and `cluster-d`
need no such mirroring, since each already lives in the same namespace as
its own server's CA.

Separately, every Klio server's **own** TLS identity, and the shared OTel
collector's own TLS identity, are signed by one cluster-wide root CA (see
`base/root_ca.yaml`), backing a `ClusterIssuer` reachable from any
namespace — a distinct trust chain from the per-server client-cert CAs
above, used only to identify servers/the collector, never to validate
clients. The root CA's own secret lives in the `cert-manager` namespace,
matching where cert-manager looks for a `ClusterIssuer`'s secret by
default.

`klio-b`'s own certificate needs an exact copy in `team-c`: Kopia
(`connection.go`) validates a Klio server's certificate by exact
fingerprint, not by CA chain, regardless of who signed it, so
kubernetes-reflector mirrors `klio-b-tls` *whole*, private key included,
even though only the public half is ever used there.

The OTel collector's own certificate needs no copying: since it's issued
by the root CA, OTel's exporters validate it through normal CA-chain
trust, so `team-c`/`team-d` only need the root CA's *public* certificate,
delivered by trust-manager.

## Prerequisites

A running Kubernetes cluster with the following operators installed:

- CloudNativePG
- Klio
- cert-manager
- kubernetes-reflector
- trust-manager
- OpenTelemetry
- Prometheus

`multi`'s `cluster-c` is a client of `klio-b`, whose CA/Issuer lives in
`default`; kubernetes-reflector mirrors the resulting client certificate
and `klio-b`'s own TLS certificate into `team-c` (Kopia validates the
latter by exact fingerprint, so an exact copy is unavoidable). trust-manager
delivers the root CA's public certificate into `team-c`/`team-d` as the
OTel collector's trust anchor. See the next section for both install
commands.

## Deploying a Kubernetes cluster with the required operators

Assuming a Kubernetes cluster with CloudNativePG and cert-manager already
installed (e.g. via the CloudNativePG `hack/setup-cluster.sh` script),
deploy Klio:

```shell
KIND_CLUSTER_NAME=$(kind get clusters | grep pg-operator-e2e) task integration:deploy-to-kind
```

Then install everything else this sample needs: kubernetes-reflector
(mirrors `cluster-c`'s client certificate and `klio-b`'s own TLS
certificate from `default` into `team-c`), trust-manager (restricted to
reading source secrets only from `cert-manager`, where the root CA's
secret lives, with Secret targets enabled and authorized for the one
secret this sample distributes), the OpenTelemetry operator, and
Prometheus (via the CloudNativePG example configuration):

```shell
helm repo add emberstack https://emberstack.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm upgrade --install reflector emberstack/reflector

helm upgrade trust-manager oci://quay.io/jetstack/charts/trust-manager \
  --install --namespace cert-manager --wait \
  --set app.trust.namespace=cert-manager \
  --set secretTargets.enabled=true \
  --set secretTargets.authorizedSecrets='{otel-collector-ca}'

kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml

helm upgrade --install \
  -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/main/docs/src/samples/monitoring/kube-stack-config.yaml \
  prometheus-community prometheus-community/kube-prometheus-stack
```

## Deploying the "single" sample

```shell
kubectl apply -k operator/config/samples/opentelemetry/single
```

Wait for `klio` and `cluster-example` to become ready, then trigger a base
backup so the Grafana dashboard has backup/snapshot data to show:

```shell
kubectl apply -f operator/config/samples/opentelemetry/single/backups-example.yaml
```

## Deploying the "multi" sample

```shell
kubectl apply -k operator/config/samples/opentelemetry/multi
```

This deploys `default` (klio-a/cluster-a, klio-b/cluster-b), `team-c`
(cluster-c) and `team-d` (a second, independent klio-a, and cluster-d) in
one shot.

## Validating the Grafana dashboard with the "multi" sample

Trigger one base backup per cluster (needed before any backup/snapshot panel
has data to show):

```shell
kubectl apply -f operator/config/samples/opentelemetry/multi/backups-example.yaml
```

Once all backups complete and Prometheus has scraped a metrics-collection
cycle, open the Klio Grafana dashboard and confirm:

- The `$namespace` variable offers `default`, `team-c` and `team-d`.
- The `$server` variable offers only `klio-a-klio-0` and `klio-b-klio-0`
  (the value is each server's pod hostname): Grafana's variable query
  dedupes identical label values, so the two independent `klio-a` servers
  (`default` and `team-d`) collapse into a single, ambiguous entry — this
  is the disambiguation gap described above, not a deployment error.
- The `$cluster` variable offers `cluster-a`, `cluster-b`, `cluster-c` and
  `cluster-d`, and narrows correctly when `$namespace`/`$server` are
  filtered (e.g. selecting `$namespace=team-c` should only ever offer
  `cluster-c`).
- Per-cluster and per-server panels correctly attribute data instead of
  aggregating everything together, and in particular that the two
  same-named `klio-a` servers (in `default` and in `team-d`) are not
  conflated.

Any panel that fails to distinguish between clusters/servers/namespaces
here is a dashboard bug to file separately; this sample's job is only to
make that determination possible.
