---
sidebar_position: 2
---

# Running E2E tests

This guide explains how to run the end-to-end (E2E) tests for the
Klio project. The E2E tests validate the integration between Klio
components and CloudNativePG in a real Kubernetes environment.

## Prerequisites

Before running E2E tests, ensure you have the following tools installed:

- [Docker](https://docs.docker.com/get-docker/) - Container runtime
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/) - Kubernetes in Docker
- [Task](https://taskfile.dev/installation/) - Task runner
- [Go](https://golang.org/doc/install) - Go programming language
- [kubectl](https://kubernetes.io/docs/tasks/tools/) - Kubernetes command-line tool

## Required Setup

:::info
Before running any E2E tests, you must first set up the CloudNativePG environment.
:::

### 1. Clone CloudNativePG Repository

```bash
git clone https://github.com/cloudnative-pg/cloudnative-pg.git
cd cloudnative-pg
```

### 2. Create Kind Cluster with CloudNativePG

```bash
# Create, load, and deploy the CloudNativePG environment
hack/setup-cluster.sh create load deploy
cd ..
```

This script will:

- Create a Kind cluster with the correct configuration
- Load the CloudNativePG operator images
- Deploy the CloudNativePG operator to the cluster

## Test Environment Setup

The E2E tests require a specific environment setup that includes:

1. **Kind cluster** with CloudNativePG operator installed ✅ (completed above)
1. **Klio operator** deployed to the cluster
1. **cert-manager** for TLS certificate management

### Automated Setup with Task

After completing the required setup above, you can use the Task
runner to deploy Klio and run tests:

```bash
# Set the Kind cluster name (must match the pattern used by CloudNativePG)
export KIND_CLUSTER_NAME=pg-operator-e2e-v1-33-1

# Run the complete E2E test suite
task integration:e2e
```

This command will:

- Deploy cert-manager to the existing Kind cluster
- Build and deploy the Klio operator
- Run all E2E tests

### Manual Setup

If you prefer to set up the environment manually or need more
control, after completing the required CloudNativePG setup above:

#### 1. Deploy Klio Operator

```bash
# Set the cluster name (check with: kind get clusters)
export KIND_CLUSTER_NAME=pg-operator-e2e-v1-33-1

# Deploy the operator to the Kind cluster
task integration:deploy-to-kind
```

#### 2. Run E2E Tests

```bash
# Run the tests
cd operator/test/e2e
go test -v ./...
```

## Test Structure

The E2E tests are located in `operator/test/e2e/` and include:

- **`main_test.go`** - Test setup, configuration, and feature
  registration
- **`common_test.go`** - Shared test utilities and helpers
- **`backup_test.go`** - Backup functionality tests:
   - `BackupFromPrimary`: backup from a single-instance cluster
   - `BackupFromStandby`: backup from a standby in a multi-instance
    cluster
- **`recovery_test.go`** - Cluster recovery tests:
   - `RecoverClusterFromBackupID`: recovery using a specific backup ID
   - `RecoverClusterFromLatestBackup`: recovery from the latest backup
   - `RecoverClusterFromPitr`: point-in-time recovery (tier1)
   - `RecoverReplicaCluster`: replica cluster creation from backup
- **`tablespace_recovery_test.go`** - Recovery preserving PostgreSQL
  tablespaces (`RecoverClusterWithTablespaces`)
- **`tier2_recovery_test.go`** - Recovery from tier2 S3 storage
  (`RecoverClusterFromTier2`)
- **`tier2_pitr_test.go`** - Point-in-time recovery from tier2 storage
  (`RecoverClusterFromTier2Pitr`)
- **`tier2_retention_test.go`** - Backup and WAL retention policy
  enforcement in tier2 storage (`Tier2Retention`)
- **`wal_retention_test.go`** - WAL retention queue-awareness: verifies
  WALs pending tier2 transfer are not prematurely deleted
  (`WALRetentionQueueAwareness`)
- **`server_reconfig_test.go`** - Adding tier2 storage to an existing
  tier1+queue server (`ServerTierReconfiguration`)
- **`pluginconfiguration_update_test.go`** - PluginConfiguration updates
  and sidecar restart behavior (`PluginConfigurationUpdate`)
- **`pvc_resize_test.go`** - PVC resize for data, cache, and queue
  volumes (`PVCResize`)
- **`otel_test.go`** - OpenTelemetry metrics and traces export: deploys
  an OTEL Collector and verifies that backup lifecycle metrics and
  traces are correctly exported via OTLP. After the success-path
  assertions it deletes the Klio server and triggers a failing backup to
  verify the `failure_category=repository_error` attribute on
  `klio.plugin.backup.runs` (`OTELMetricsAndTraces`)
- **`operator_otel_test.go`** - Operator OpenTelemetry metrics:
  patches the operator deployment with OTEL env vars and verifies
  that controller-runtime metrics are bridged to an OTLP collector
  (`OperatorOTELMetrics`, serial)

## Test Configuration

The e2e tests are configured through a YAML file located at
`operator/test/e2e/e2e-config.yaml`. The file is committed with
default values that match the local development environment.

Available options:

```yaml
# Klio server container image used in e2e tests.
serverImage: "registry.dev:5000/klio-testing:dev"

# Directory where pod logs are streamed during the test run.
logDir: "e2e_cluster_logs"

# Kubernetes storage class used for all PVC templates in the tests.
storageClass: "csi-hostpath-sc"

# Namespace where the Klio operator runs. Defaults to cnpg-system (Helm/Kind);
# set to openshift-operators for the OLM-based OpenShift install.
operatorNamespace: "cnpg-system"

# Label selector for identifying the Klio operator deployment. Must match
# the labels applied in the operator deployment manifest.
operatorAppLabel: "app.kubernetes.io/name=klio"

# Name of the OLM Subscription managing the operator (in operatorNamespace).
# Set it on OpenShift so tests that change the operator environment patch the
# Subscription (which OLM propagates to the Deployment) rather than the
# Deployment, which OLM reverts. Leave empty for the Helm/Kind install.
operatorSubscription: ""

# Registry credentials for pulling private images.
# The secret is created automatically in every test namespace.
# Leave all fields empty when images are publicly accessible.
# Do NOT commit credentials — use a local, untracked copy of this
# file or set them via environment variables instead.
imagePullSecret:
  registry: ""
  username: ""
  password: ""
```

To use a custom configuration, either edit `e2e-config.yaml` directly
or point the `E2E_CONFIG_FILE` environment variable at an alternative
file:

```bash
E2E_CONFIG_FILE=/path/to/my-config.yaml go test -v ./...
```

Configuration is resolved in two layers: built-in defaults, then the
config file. Pointing `E2E_CONFIG_FILE` at a personal file outside the
repository is the recommended way to keep local overrides (including
credentials) out of version control.

## Log Collection

During a test run, the test suite streams logs from all relevant pods in
the cluster using stern. Logs are written to a directory on the local
filesystem. The following components are covered:

- Klio operator
- CNPG operator
- PostgreSQL instances
- RustFS server and init-job pods
- Klio server pods

The log directory is controlled by the `logDir` key in the config file
(default: `e2e_cluster_logs/` relative to the test working directory).

When running via `task integration:e2e`, logs are exported to
`e2e_cluster_logs/` in the repository root after the test run
completes. In CI, that directory is uploaded as a workflow artifact
named `e2e_cluster_logs` and is available for download from the
GitHub Actions run page.

### Namespace object dumps on failure

When a feature fails, its teardown writes a JSON snapshot of the
objects in the test namespace under `<logDir>/<namespace>/`, one
`<Kind>.json` file per resource kind (for example `PodList.json` and
`ClusterList.json`), before the namespace is deleted. It covers the Klio
and CloudNativePG custom resources together with the core workload
objects (Pods, PVCs, Jobs, Events, ServiceAccounts, EndpointSlices,
StatefulSets and Deployments), giving a point-in-time view of the
cluster state at the moment of failure.

## Running on OpenShift

The project includes an OpenShift-based e2e pipeline that validates
Klio on a single-node OpenShift (SNO) cluster. The cluster is
provisioned with [CRC](https://github.com/crc-org/crc-github-action)
(CodeReady Containers) using the `openshift` preset, which already
ships OLM, the operator catalogs, and a default StorageClass. The
pipeline then installs cert-manager, community CNPG, and the Klio
operator via OLM subscriptions and runs the same Go e2e suite as the
Kind path.

### OpenShift prerequisites

CRC runs the cluster inside a KVM virtual machine, so the runner must
expose `/dev/kvm` (nested virtualization). In addition to Go and Task,
the pipeline requires:

- A pre-built Klio OLM catalog artifact
  (`operator/klio-operator-catalog-source.yaml`)
- A Red Hat pull secret (`REDHAT_REGISTRY_DOCKERCONFIG`) used to start
  the CRC cluster
- GHCR credentials (`GHCR_USERNAME`, `GHCR_TOKEN`) for the private Klio
  images

### Running locally

Start a CRC `openshift` cluster yourself, then point the tasks at its
kubeconfig (CRC writes it to `~/.kube/config`):

```bash
export EXTERNAL_KUBECONFIG="$HOME/.kube/config"
export GHCR_USERNAME=<github-user>
export GHCR_TOKEN=<github-token>

# Generate the Klio OLM CatalogSource the deploy step requires (or
# download the klio-olm artifact from the ci.yml olm job instead).
task olm:catalog-source

task integration:e2e-openshift
```

This command will:

- Add GHCR credentials to the cluster-wide pull secret
- Deploy cert-manager, CNPG, and Klio via OLM subscriptions
- Run the full e2e suite

> **Storage note:** CRC's default StorageClass is hostPath-backed and
> cannot expand volumes, so `task integration:e2e-openshift` deploys
> the upstream `csi-driver-host-path` (the same driver the Kind path
> uses) via its `integration:setup-crc-storage` dependency and points
> `storageClass` at `csi-hostpath-sc`, which has `allowVolumeExpansion`
> enabled for the `PVCResize` feature. This runs the same way in CI and
> locally; override `STORAGE_CLASS` to use another expandable class.

### CI

The OpenShift e2e job does not run on regular pull requests. It runs
on pushes to `main`, on the `release-please--branches--main` release
PR, and on demand via the CI workflow's `workflow_dispatch`
(`run-openshift` input). When it runs, the `core` and `operator` jobs
are forced to build so their images are published to GHCR before the
suite pulls them; the deploy step also consumes the `klio-olm`
CatalogSource artifact produced by the always-on `olm` job. It is
defined in `.github/workflows/openshift-e2e.yml` and provisions the
cluster with the `crc-org/crc-github-action` action.

## Environment Variables

The following environment variables can be used to customize the
tasks execution:

- `KIND_CLUSTER_NAME` - Name of the Kind cluster to use (required
  for the Kind path)
- `E2E_CONFIG_FILE` - Path to a custom test configuration file
  (default: `operator/test/e2e/e2e-config.yaml`)

## Writing New Tests

When adding new E2E tests:

1. Follow the existing patterns in `backup_test.go` or
   `recovery_test.go`
1. Use the machinery framework for test setup
   1. Add new feature types if needed. Everything in the framework must be
       plugin-agnostic, and can be tested only through CloudNativePG APIs.
1. Register new features in `main_test.go`
1. Ensure proper cleanup of test resources

## CI Integration

The E2E tests are automatically run in CI when:

- Changes are made to core components
- Changes are made to operator components
- Pull requests are submitted

The CI environment automatically handles cluster setup and cleanup.
After the run, cluster logs are uploaded as a workflow artifact named
`e2e_cluster_logs` and can be downloaded from the GitHub Actions run
page regardless of whether the tests passed or failed.
