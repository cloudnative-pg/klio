# README

This directory contains two sample environments exercising Klio's
OpenTelemetry integration, both sharing the OTel collector / Jaeger /
Prometheus stack defined in `base/`:

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

## Prerequisites

A running Kubernetes cluster with the following operators installed:

- CloudNativePG
- Klio
- cert-manager
- OpenTelemetry
- Prometheus

`jq` must also be available locally (used by `multi/copy-cross-namespace-secrets.sh`
and `multi/bootstrap-remote-server.sh`).

All of `multi`'s client certificates (cluster-a's through cluster-d's) are
issued through a `ClusterIssuer`, which resolves its backing CA secret in
cert-manager's `--cluster-resource-namespace` rather than in the namespace
of the `Certificate` requesting it. This matters only for cluster-c's and
cluster-d's, which request their certificate from `team-c`/`team-d`
directly instead of `default` — cluster-a's and cluster-b's already live in
`default`, so the same lookup is a same-namespace no-op for them. This
sample assumes cert-manager's cluster resource namespace is `default`
(where `base/klio_server_ca.yaml` is deployed), so install cert-manager
accordingly, e.g. with the Jetstack Helm chart:

```shell
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true \
  --set clusterResourceNamespace=default
```

## Deploying a Kubernetes cluster with the required operators

Assuming an environment with CloudNativePG, Klio and cert-manager
created through the CloudNativePG `hack/setup-cluster.sh` script and
the klio task

```shell
KIND_CLUSTER_NAME=$(kind get clusters | grep pg-operator-e2e) task integration:deploy-to-kind
```

you can install the OpenTelemetry operator by running:

```shell
kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml
```

You can install Prometheus using the Prometheus community Helm chart and
the CloudNativePG example configuration:

```shell
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
```

```shell
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

1. Deploy the two servers, the two same-namespace clusters (cluster-a,
   cluster-b) and the shared OTel/Jaeger stack, all in `default`:

   ```shell
   kubectl apply -k operator/config/samples/opentelemetry/multi
   ```

   Wait for `klio-a`, `klio-b`, `cluster-a` and `cluster-b` to become ready
   before continuing.

1. cluster-c's own client certificate is requested directly in `team-c`
   through the cluster-scoped `klio-server-ca` `ClusterIssuer`, so it needs
   no copying. klio-b's server certificate and the OTel collector's
   certificate are pinned by exact bytes rather than CA-validated (see the
   script's comments), so those still have to be copied from `default`
   into the `team-c` namespace:

   ```shell
   ./operator/config/samples/opentelemetry/multi/copy-cross-namespace-secrets.sh
   ```

1. Deploy cluster-c into `team-c`:

   ```shell
   kubectl apply -k operator/config/samples/opentelemetry/multi/team-c
   ```

1. cluster-d's own client certificate is likewise requested directly in
   `team-d` through the `klio-server-ca` `ClusterIssuer`. `team-d`'s server
   (its own "klio-a") is independently self-signed rather than a copy of
   `default`'s CA-issued certificate, but it still needs to validate
   clients signed by the shared `klio-server-ca` and to export telemetry to
   the shared collector. Copy the CA's public certificate and the OTel
   collector's trust anchor into the `team-d` namespace:

   ```shell
   ./operator/config/samples/opentelemetry/multi/bootstrap-remote-server.sh team-d
   ```

1. Deploy cluster-d (and its own klio-a server) into `team-d`:

   ```shell
   kubectl apply -k operator/config/samples/opentelemetry/multi/team-d
   ```

## Validating the Grafana dashboard with the "multi" sample

Trigger one base backup per cluster (needed before any backup/snapshot panel
has data to show):

```shell
kubectl apply -f operator/config/samples/opentelemetry/multi/backups-example.yaml
```

Once all backups complete and Prometheus has scraped a metrics-collection
cycle, open the Klio Grafana dashboard and confirm:

- The `$namespace` variable offers `default`, `team-c` and `team-d`.
- The `$server` variable offers `klio-a-klio-0` (twice, once per namespace)
  and `klio-b-klio-0` (the value is each server's pod hostname).
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