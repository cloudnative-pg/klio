# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Project overview

Klio is an enterprise-grade backup and recovery manager for PostgreSQL databases.
It has special integrations for CloudNativePG on Kubernetes, but should
work with any PostgreSQL setup.
It consists of two main Go modules in a monorepo structure.

## Build System

This project uses [Task](https://taskfile.dev/) (not Make) as the primary build system. The main `Taskfile.yml` is in the repository root.

### Common Commands

```bash
# Run complete CI pipeline
task all:ci

# Core module
task core:lint              # Run golangci-lint on core
task core:go-test           # Run unit tests
task core:protoc-gen-go-grpc  # Compile proto files
task core:openapi-gen       # Generate OpenAPI specification

# Operator module
task operator:lint          # Run golangci-lint on operator
task operator:controller-gen  # Generate CRDs and DeepCopy methods
task operator:helm-chart    # Generate Helm chart

# Documentation
task documentation:ci       # Run documentation CI
task documentation:spellcheck  # Run spellcheck

# Integration testing
task integration:devenv     # Create ephemeral development environment
task integration:e2e        # Run e2e tests (requires KIND_CLUSTER_NAME)

# Cleanup
task clean:all              # Full cleanup of generated elements
```

### Key Custom Resources

- **Server**: Deploys the Klio backup server (StatefulSet with PVCs for cache, data, queue)
- **PluginConfiguration**: Configures the CloudNativePG plugin for a cluster

### Build Pipeline

The CI uses Dagger for containerized builds. Key tasks in `Taskfile.yml`:
- Proto compilation via `cloudnative-pg/daggerverse/protoc-gen-go-grpc`
- Linting via `sagikazarmark/daggerverse/golangci-lint`
- Controller-gen via `cloudnative-pg/daggerverse/controller-gen`
- Container builds via `docker buildx bake`

### Unit Tests

```bash
# Core tests (uses envtest for Kubernetes)
task core:go-test

# Operator tests
cd operator && go test ./... -v
```

#### Running a Single Test
```bash
# Core module
cd core && go test -v -run TestFunctionName ./path/to/package

# Operator module
cd operator && go test -v -run TestFunctionName ./path/to/package
```

### E2E Tests
```bash
# Requires a Kind cluster created by CNPG hack/setup-cluster.sh
KIND_CLUSTER_NAME=$(kind get clusters  | grep pg-operator-e2e) task integration:e2e
```

## Code Style

- Avoid inline error strings; define error variables instead
  (e.g., `var ErrSomething = errors.New("message")`)
- Comments on exported functions and variables must end with a period.

# Important notes

- These files must be kept in sync:
  - `operator/pkg/config/server.go` ↔ `client/pkg/config/server.go`
  - `operator/pkg/config/client.go` ↔ `client/pkg/config/client.go`

## PR instructions

- Title format: conventional commit
- Commits must have a `Signed-off-by` footer
- Before committing, run:
  - `golangci-lint run` in `core/`
  - `golangci-lint run` in `operator/`
  - `dagger call all-ci`

## Key Dependencies

- CloudNativePG API and machinery (`github.com/cloudnative-pg/*`)
- Kubernetes controller-runtime (`sigs.k8s.io/controller-runtime`)
- NATS for the task queue (`github.com/nats-io/nats.go`)
- Kopia for deduplication (forked at `github.com/leonardoce/kopia`)
- gRPC for client-server communication

## Documentation

Documentation site is in `documentation/web/` (Docusaurus):
```bash
cd documentation/web
docker run -ti --rm -v $(pwd):/website -w /website --net host node:24 bash -c "yarn && yarn start" # Development server on localhost:3000
```

`md` and `mdx` files in the documentation should have a maximum line length of
80 characters.

Official docs: https://enterprisedb.github.io/klio
