# README

This directory contains a sample environment with **two Klio servers** and
**three CloudNativePG clusters** distributed across them, one of which lives
in a different namespace than the servers. It exercises the same
OpenTelemetry + Prometheus + Grafana stack as
`operator/config/samples/opentelemetry/`, but is meant to validate the
Grafana dashboard's `$namespace`, `$server` and `$cluster` template
variables and its per-tier/per-cluster aggregations against more than one
server or cluster, something the single-server sample can't exercise.

## Topology

| Cluster     | Namespace | Backed by |
|-------------|-----------|-----------|
| cluster-a   | default   | klio-a    |
| cluster-b   | default   | klio-b    |
| cluster-c   | team-c    | klio-b    |

`klio-b` intentionally backs clusters in two different namespaces, since a
single shared backup server serving multiple application namespaces is a
realistic multi-tenant deployment and the case most likely to expose
dashboard attribution bugs.

## Prerequisites

A running Kubernetes cluster with the following operators installed:

- CloudNativePG
- Klio
- cert-manager
- OpenTelemetry
- Prometheus

`jq` must also be available locally (used by `copy-cross-namespace-secrets.sh`).

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

## Deploying the sample configuration

1. Deploy the two servers, the two same-namespace clusters (cluster-a,
   cluster-b) and the shared OTel/Jaeger stack, all in `default`:

   ```shell
   kubectl apply -k operator/config/samples/opentelemetry-multi
   ```

   Wait for `klio-a`, `klio-b`, `cluster-a` and `cluster-b` to become ready
   before continuing.

1. cert-manager `Issuer`s are namespace-scoped, so the client certificate
   cluster-c needs (to authenticate to klio-b) can only be generated in
   `default`, where the `klio-server-ca` Issuer lives. Copy that
   certificate, plus the (pinned, not CA-validated — see the script's
   comments) server and OTel collector certificates, into the `team-c`
   namespace:

   ```shell
   ./operator/config/samples/opentelemetry-multi/copy-cross-namespace-secrets.sh
   ```

1. Deploy cluster-c into `team-c`:

   ```shell
   kubectl apply -k operator/config/samples/opentelemetry-multi/team-c
   ```

## Validating the Grafana dashboard

Trigger one base backup per cluster (needed before any backup/snapshot panel
has data to show):

```shell
kubectl apply -f operator/config/samples/opentelemetry-multi/backups-example.yaml
```

Once all three backups complete and Prometheus has scraped a
metrics-collection cycle, open the Klio Grafana dashboard and confirm:

- The `$namespace` variable offers both `default` and `team-c`.
- The `$server` variable offers both `klio-a-klio-0` and `klio-b-klio-0`
  (the value is each server's pod hostname).
- The `$cluster` variable offers `cluster-a`, `cluster-b` and `cluster-c`,
  and narrows correctly when `$namespace`/`$server` are filtered (e.g.
  selecting `$namespace=team-c` should only ever offer `cluster-c`).
- Per-cluster and per-server panels correctly attribute data instead of
  aggregating everything together.

Any panel that fails to distinguish between clusters/servers/namespaces
here is a dashboard bug to file separately; this sample's job is only to
make that determination possible.
