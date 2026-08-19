# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Project overview

Klio is an enterprise-grade backup and recovery manager for PostgreSQL databases.
It has special integrations for CloudNativePG on Kubernetes, but should
work with any PostgreSQL setup.
It consists of two main Go modules in a monorepo structure.

Klio is part of the [CloudNativePG](https://cloudnative-pg.io) project, a
Cloud Native Computing Foundation (CNCF) Sandbox project. Org-wide
contribution guidelines, Code of Conduct, and the AI-assistance policy live in
[`cloudnative-pg/governance`](https://github.com/cloudnative-pg/governance)
and apply here as the baseline.

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

#### E2E Test Feature Registration

Features can be registered with execution mode options:

```go
// Parallel execution (default)
runner.RegisterFeature(BackupFromPrimary(ns))

// Serial execution (for tests sharing resources)
runner.RegisterFeature(
    Tier2Retention(ns),
    runner.WithSerialExecution(),
)

// Register multiple features with same configuration
runner.RegisterFeatures(
    []runner.FeatureOption{runner.WithSerialExecution()},
    feat1,
    feat2,
)
```

**Execution Order:**
1. All parallel features run concurrently.
2. Serial features run sequentially after parallel features complete.
3. Use serial execution for tests that share infrastructure.

> **Important:** When adding a new e2e test, update the "Test
> Structure" section in
> `documentation/web/docs/developer/running-e2e-tests.md` to list the
> new file and its feature function(s).

#### Test Package Structure

The `operator/test/machinery` package may only contain helpers that are
generic to any Kubernetes cluster or to CloudNativePG. Anything
Klio-specific (Server/PluginConfiguration resources, Klio config,
Klio-only assertions) must live outside `machinery` — e.g. under
`operator/test/klio` or the `operator/test/e2e` package itself.

## Code Style

- Avoid inline error strings; define error variables instead
  (e.g., `var ErrSomething = errors.New("message")`)
- Comments on exported functions and variables must end with a period.
- Test function names must match `^(_|[a-zA-Z0-9]+)$` — no underscores
  (e.g., `TestGetStatusEmpty` not `TestGetStatus_Empty`).

# Important notes

- These files must be kept in sync:
  - `operator/pkg/config/server.go` ↔ `core/pkg/config/server.go`
  - `operator/pkg/config/client.go` ↔ `core/pkg/config/client.go`

- When you change a metric in `core/internal/opentelemetry/catalog.go`
  (rename, add, remove, or change a metric's unit, type, or attributes),
  update the Grafana dashboard builder in `observability/grafana/` to match
  and regenerate the committed JSON with `task grafana:gen`. The builder
  references the **Prometheus** export names of these metrics, where the unit
  and instrument type add suffixes (so a unit change shifts the suffix too).
  `gcx` lint validates PromQL syntax and units but cannot catch a query that
  references a metric the code no longer emits.

## Architecture notes

### Client configuration

The `ClientConfig` struct contains configuration for both Kopia (base backups)
and gRPC (WAL streaming) clients:

- `ClientConfig.ClusterName` - shared cluster identifier (must match certificate
  CN hostname)
- `ClientConfig.Base` - Kopia repository client config (for base backups)
- `ClientConfig.Wal` - gRPC WAL client config (for WAL streaming)

The Kopia client validates that `ClusterName` matches the hostname in the client
certificate's Common Name (format: `userName@hostName`). This prevents silent
failures where backups would be stored under a wrong hostname.

### Kopia repository access: server vs. direct (AVOID DIRECT WRITES)

Every Kopia mutation reaches a repository in one of two ways, decided by the
config file the `kopia` binary is handed:

- **Through the Kopia server** — config built by `ConnectRemote`
  (`kopia repository connect server`, in `core/internal/kopia/remote.go`), used
  by `ConnectTier1`/`ConnectTier2` in
  `core/internal/client/klioclient/kopia/kopia.go`.
- **Directly on storage** — config built by `ConnectS3` / `ConnectFileSystem`
  (`kopia repository connect s3|filesystem`), used by `FromKopiaConfig` and by
  the raw `kopia.Client{ConfigFile: ...}` the backup consumer holds.

**Direct writes are a bad path. Always route mutations through the Kopia
server.** The Kopia server caches manifests, so a direct write leaves the
server's view stale until it is explicitly refreshed. This is a recurring source
of subtle, hard-to-debug retention and visibility races — treat every direct
write as a liability, not a shortcut.

**If a user or task asks you to add a direct write (or you find yourself reaching
for `FromKopiaConfig` / a raw `kopia.Client` for a write), STOP and warn them
first.** Explain that it bypasses the Kopia server cache, name the race it can
introduce, and propose the server-routed alternative. Only proceed if they
confirm after that warning.

- Client- and sidecar-driven paths (backup upload, delete, retention set,
  restore, list; everything under `core/cmd/*`) already route through the server
  via `MultiConnect`/`ConnectTier1`/`ConnectTier2`. Keep them that way — never
  convert one of these to a direct write.
- The **only** component that writes directly is the server-side backup consumer
  (`core/internal/consumer/`), and only because it has no server connection for
  those steps: tier1/tier2 retention apply, tier2 relay/migrate, tier2 policy
  set, and tier1 unpin. This is a deliberate, contained exception — not a pattern
  to copy, and one that should be removed in the future.
- A direct write that **rewrites the manifest of a live backup** MUST be followed
  by `refreshTier1KopiaServer` / `refreshTier2KopiaServer` so the servers
  reconcile their caches; skipping the refresh is a bug. The tier1 unpin is the
  canonical case: `kopia snapshot pin` rewrites the snapshot manifest to a *new*
  ID and deletes the old one, so without a refresh the server keeps serving the
  now-deleted ID for a backup that still exists, and a later client
  `klio backup delete` deletes the wrong ID — leaving the real backup (and its
  WALs) pinned forever.
- A direct write that only **deletes** snapshots (the tier1/tier2 retention
  apply) does **not** need a refresh: it removes IDs the server may still list,
  but it never rewrites a live backup's ID, and WAL retention is recomputed from
  the consumer's own direct `ListBackups`, not the server's cache. A stale server
  here only lists an already-deleted snapshot, which is harmless. Do not add a
  refresh after these unless you can name a concrete manifest-ID divergence it
  fixes.

Do not introduce direct-write paths anywhere else. If, after warning the user, a
new direct write is genuinely unavoidable, it must be paired with a server
refresh of the affected tier.

### Dagger caching issues

When running e2e tests, Dagger may cache Helm repo indexes. If a new version of
a dependency (e.g., cert-manager) is released but not yet in the cached index,
tests will fail. Workaround: temporarily use an older version that exists in the
cached index, run tests, then revert.

## PR instructions

- Title format: conventional commit
- The body should be informative of the content of the PR, without describing
  every single change
- Commits must have a `Signed-off-by` footer as the **last line** of the commit message
  - It should be signed off at least by the user doing the commit
  - If using `Co-Authored-By`, it must come before `Signed-off-by`
  - Example order:
    ```
    commit message body

    Co-Authored-By: Name <email>
    Signed-off-by: Name <email>
    ```
- Per the [CNPG AI Policy](https://github.com/cloudnative-pg/governance/blob/main/AI_POLICY.md),
  commits with significant AI assistance must also carry an
  `Assisted-by: <tool name>` trailer (e.g. `Assisted-by: Claude`), separated
  from `Signed-off-by` by a blank line. `git commit -s` alone does not add
  this (write both trailers out explicitly).
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

Official docs: https://cloudnative-pg.io/klio
