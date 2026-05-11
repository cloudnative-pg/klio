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

:::important
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
  traces are correctly exported via OTLP (`OTELMetricsAndTraces`)

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

## Environment Variables

The following environment variables can be used to customize the tasks
execution:

- `KIND_CLUSTER_NAME` - Name of the Kind cluster to use (required)
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
