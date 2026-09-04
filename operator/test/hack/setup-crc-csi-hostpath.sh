#!/usr/bin/env bash
##
## Copyright © contributors to CloudNativePG, established as
## CloudNativePG a Series of LF Projects, LLC.
##
## Licensed under the Apache License, Version 2.0 (the "License");
## you may not use this file except in compliance with the License.
## You may obtain a copy of the License at
##
##     http://www.apache.org/licenses/LICENSE-2.0
##
## Unless required by applicable law or agreed to in writing, software
## distributed under the License is distributed on an "AS IS" BASIS,
## WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
## See the License for the specific language governing permissions and
## limitations under the License.
##
## SPDX-License-Identifier: Apache-2.0
##

# Deploy the upstream kubernetes-csi/csi-driver-host-path so the e2e suite has
# an expandable StorageClass (csi-hostpath-sc), exactly as the Kind path does
# (CNPG's setup-cluster.sh installs the same driver there). CRC's default
# crc-csi-hostpath-provisioner (kubevirt HPP) cannot expand PVCs, which the
# PVCResize feature requires.
set -euo pipefail

# renovate: datasource=github-releases depName=kubernetes-csi/csi-driver-host-path
CSI_DRIVER_HOST_PATH_VERSION="v1.18.0"

# csi-hostpath-plugin.yaml bundles every sidecar into one pod and ships the
# ClusterRoleBindings for csi-hostpathplugin-sa, but NOT the ClusterRoles they
# bind to (upstream's deploy-hostpath.sh pulls those from each sidecar repo).
# Without them the bindings dangle, the SA gets no permissions, and the
# external-provisioner cannot list PVCs/Nodes/StorageClasses — leaving every
# csi-hostpath-sc PVC stuck Pending. These versions track the sidecar images in
# csi-driver-host-path ${CSI_DRIVER_HOST_PATH_VERSION}.
# renovate: datasource=github-releases depName=kubernetes-csi/external-provisioner
EXTERNAL_PROVISIONER_VERSION="v6.3.0"
# renovate: datasource=github-releases depName=kubernetes-csi/external-attacher
EXTERNAL_ATTACHER_VERSION="v4.13.0"
# renovate: datasource=github-releases depName=kubernetes-csi/external-resizer
EXTERNAL_RESIZER_VERSION="v2.2.1"
# renovate: datasource=github-releases depName=kubernetes-csi/external-snapshotter
EXTERNAL_SNAPSHOTTER_VERSION="v8.6.0"
# renovate: datasource=github-releases depName=kubernetes-csi/external-health-monitor
EXTERNAL_HEALTH_MONITOR_VERSION="v0.18.0"

workdir=$(mktemp -d)
trap 'rm -rf "${workdir}"' EXIT

git clone --depth 1 --branch "${CSI_DRIVER_HOST_PATH_VERSION}" \
    https://github.com/kubernetes-csi/csi-driver-host-path "${workdir}/csi"

manifests="${workdir}/csi/deploy/kubernetes-latest/hostpath"

# Deploy the CSIDriver and the csi-hostpathplugin StatefulSet (which bundles the
# external provisioner/attacher/resizer sidecars) into the default namespace.
# Skip csi-hostpath-testing.yaml (a socat helper we do not need).
kubectl apply \
    -f "${manifests}/csi-hostpath-driverinfo.yaml" \
    -f "${manifests}/csi-hostpath-plugin.yaml"

# ClusterRoles the bundled sidecars' bindings reference (see the version block
# above). Each rbac.yaml also creates a per-sidecar SA + binding we do not use;
# those are harmless. Without these the provisioner has no RBAC and PVCs never
# bind.
base="https://raw.githubusercontent.com/kubernetes-csi"
kubectl apply \
    -f "${base}/external-provisioner/${EXTERNAL_PROVISIONER_VERSION}/deploy/kubernetes/rbac.yaml" \
    -f "${base}/external-attacher/${EXTERNAL_ATTACHER_VERSION}/deploy/kubernetes/rbac.yaml" \
    -f "${base}/external-resizer/${EXTERNAL_RESIZER_VERSION}/deploy/kubernetes/rbac.yaml" \
    -f "${base}/external-snapshotter/${EXTERNAL_SNAPSHOTTER_VERSION}/deploy/kubernetes/csi-snapshotter/rbac-csi-snapshotter.yaml" \
    -f "${base}/external-health-monitor/${EXTERNAL_HEALTH_MONITOR_VERSION}/deploy/kubernetes/external-health-monitor-controller/rbac.yaml"

# OpenShift only: the plugin pod runs privileged with hostPath mounts, so its
# ServiceAccount needs the privileged SCC (Kind has no SCCs). Grant it, then
# restart the workload so its pods are admitted under the SCC.
oc adm policy add-scc-to-user privileged \
    -z csi-hostpathplugin-sa -n default
kubectl -n default rollout restart statefulset/csi-hostpathplugin

# Expandable StorageClass named csi-hostpath-sc — the same name the Kind path
# points the e2e config at (provisioner hostpath.csi.k8s.io,
# allowVolumeExpansion: true).
kubectl apply -f "${workdir}/csi/examples/csi-storageclass.yaml"

kubectl -n default rollout status statefulset/csi-hostpathplugin --timeout=300s
kubectl get storageclass csi-hostpath-sc
