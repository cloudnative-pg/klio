#!/usr/bin/env bash
# Bootstraps a namespace that runs its OWN local Klio server (self-signed,
# not copied from "default") which nonetheless needs to: (a) validate client
# certs signed by the shared klio-server-ca, and (b) export OTel data to the
# shared collector in "default". This copies the CA's public cert so any
# locally-issued server certificate can validate against it, plus the shared
# OTel collector/client certs — all three are pinned by exact bytes rather
# than validated through a CA (see
# core/internal/client/klioclient/grpcclient/connection.go), so there is no
# ClusterIssuer shortcut for them. The destination namespace's own client
# certificate (e.g. cluster_d_klio_client_auth.yaml) needs no such copy: it
# is requested directly there via the cluster-scoped klio-server-ca
# ClusterIssuer.
#
# Usage: bootstrap-remote-server.sh <dest-namespace>
#
# Run this after `kubectl apply -k .` and before applying the destination
# namespace's kustomization, e.g.:
#   ./bootstrap-remote-server.sh team-d
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <dest-namespace>" >&2
  exit 1
fi

SOURCE_NS=default
DEST_NS="$1"

echo "Waiting for cert-manager to issue the secrets in ${SOURCE_NS}..."
kubectl wait --for=create secret/klio-server-ca -n "${SOURCE_NS}" --timeout=120s
kubectl wait --for=create secret/otel-collector-tls -n "${SOURCE_NS}" --timeout=120s
kubectl wait --for=create secret/klio-server-otel-client-tls -n "${SOURCE_NS}" --timeout=120s

kubectl create namespace "${DEST_NS}" --dry-run=client -o yaml | kubectl apply -f -

echo "Copying the shared CA's public certificate (so a locally-issued server cert can validate clients signed by it)..."
kubectl get secret klio-server-ca -n "${SOURCE_NS}" -o jsonpath='{.data.tls\.crt}' | base64 -d |
  kubectl create secret generic klio-server-ca -n "${DEST_NS}" --from-file=tls.crt=/dev/stdin \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Copying the OTel collector's certificate (public cert only, used as the trust anchor)..."
kubectl get secret otel-collector-tls -n "${SOURCE_NS}" -o jsonpath='{.data.ca\.crt}' | base64 -d |
  kubectl create secret generic otel-collector-ca -n "${DEST_NS}" --from-file=ca.crt=/dev/stdin \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Copying the shared OTel client certificate (full secret; not verified by the collector)..."
kubectl get secret klio-server-otel-client-tls -n "${SOURCE_NS}" -o json |
  jq 'del(.metadata.namespace,.metadata.resourceVersion,.metadata.uid,.metadata.creationTimestamp,.metadata.ownerReferences,.metadata.annotations)' |
  kubectl apply -n "${DEST_NS}" -f -

echo "Done. You can now apply ${DEST_NS}'s kustomization."