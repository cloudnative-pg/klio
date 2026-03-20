---
sidebar_position: 2
---

# Running E2E tests

This guide explains how to run the end-to-end (E2E) tests for the Klio project. The E2E tests validate the integration between Klio components and CloudNativePG in a real Kubernetes environment.

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
2. **Klio operator** deployed to the cluster
3. **cert-manager** for TLS certificate management

### Automated Setup with Task

After completing the required setup above, you can use the Task runner to deploy Klio and run tests:

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

If you prefer to set up the environment manually or need more control, after completing the required CloudNativePG setup above:

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

- **`main_test.go`** - Test setup and configuration
- **`backup_test.go`** - Backup functionality tests, including:
  - Backup from primary PostgreSQL instances
  - Backup from standby PostgreSQL instances

## Log Collection

During a test run, the test suite streams logs from all relevant pods in
the cluster using stern. Logs are written to a directory on the local
filesystem. The following components are covered:

- Klio operator
- CNPG operator
- PostgreSQL instances
- RustFS server and init-job pods
- Klio server pods

The log directory defaults to `e2e_cluster_logs/` relative to the
working directory. You can override it with the `E2E_LOG_DIR`
environment variable.

When running via `task integration:e2e`, logs are exported to
`e2e_cluster_logs/` in the repository root after the test run
completes. In CI, that directory is uploaded as a workflow artifact
named `e2e_cluster_logs` and is available for download from the
GitHub Actions run page.

## Environment Variables

The following environment variables can be used to customize the tasks
execution:

- `KIND_CLUSTER_NAME` - Name of the Kind cluster to use (required)
- `E2E_LOG_DIR` - Directory where cluster logs are written during the
  test run (default: `e2e_cluster_logs/`)

## Writing New Tests

When adding new E2E tests:

1. Follow the existing pattern in `backup_test.go`
2. Use the machinery framework for test setup
    1. Add new feature types if needed. Everything in the framework must be
       plugin-agnostic, and can be tested only through CloudNativePG APIs.
3. Register new features in `main_test.go`
4. Ensure proper cleanup of test resources

## CI Integration

The E2E tests are automatically run in CI when:

- Changes are made to core components
- Changes are made to operator components
- Pull requests are submitted

The CI environment automatically handles cluster setup and cleanup.
After the run, cluster logs are uploaded as a workflow artifact named
`e2e_cluster_logs` and can be downloaded from the GitHub Actions run
page regardless of whether the tests passed or failed.
