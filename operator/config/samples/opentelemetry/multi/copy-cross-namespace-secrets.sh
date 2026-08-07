#!/usr/bin/env bash
# Copies the secrets cluster-c (namespace "team-c") needs from klio-b and the
# shared OTel collector, both of which live in "default". klio-b's server
# certificate and the OTel collector's certificate are pinned by exact bytes
# rather than validated through a CA (see
# core/internal/client/klioclient/grpcclient/connection.go), so only their
# public half needs copying — there is no ClusterIssuer shortcut for these,
# unlike cluster-c's own client certificate (see
# cluster_c_klio_client_auth.yaml), which is requested directly in "team-c"
# via the cluster-scoped klio-server-ca ClusterIssuer and needs no copy.
#
# Run this after `kubectl apply -k .` and before `kubectl apply -k team-c`.
set -euo pipefail

SOURCE_NS=default
DEST_NS=team-c

echo "Waiting for cert-manager to issue the secrets in ${SOURCE_NS}..."
kubectl wait --for=create secret/klio-b-tls -n "${SOURCE_NS}" --timeout=120s
kubectl wait --for=create secret/otel-collector-tls -n "${SOURCE_NS}" --timeout=120s
kubectl wait --for=create secret/klio-server-otel-client-tls -n "${SOURCE_NS}" --timeout=120s

kubectl create namespace "${DEST_NS}" --dry-run=client -o yaml | kubectl apply -f -

echo "Copying klio-b's server certificate (public cert only, pinned by the client)..."
kubectl get secret klio-b-tls -n "${SOURCE_NS}" -o jsonpath='{.data.tls\.crt}' | base64 -d |
  kubectl create secret generic klio-b-tls -n "${DEST_NS}" --from-file=tls.crt=/dev/stdin \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Copying the OTel collector's certificate (public cert only, used as the trust anchor)..."
kubectl get secret otel-collector-tls -n "${SOURCE_NS}" -o jsonpath='{.data.ca\.crt}' | base64 -d |
  kubectl create secret generic otel-collector-ca -n "${DEST_NS}" --from-file=ca.crt=/dev/stdin \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Copying the shared OTel client certificate (full secret; not verified by the collector)..."
kubectl get secret klio-server-otel-client-tls -n "${SOURCE_NS}" -o json |
  jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid,.metadata.creationTimestamp,.metadata.ownerReferences,.metadata.annotations)' |
  kubectl apply -n "${DEST_NS}" -f -

echo "Done. You can now run: kubectl apply -k team-c"