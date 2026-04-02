# README

This directory contains sample configurations for the a klio server and a
CNPG cluster with OpenTelemetry enabled.

## Prerequisites

A running Kubernetes cluster with the following operators installed:

- CloudNativePG
- Klio
- cert-manager
- OpenTelemetry
- Prometheus

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

```shell
kubectl apply -k operator/config/samples/opentelemetry
```
