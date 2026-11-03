---
sidebar_position: 3
---

# Testing the OLM bundle

This guide explains how to build and test the Klio OLM (Operator
Lifecycle Manager) bundle. The tasks come from the OLM Taskfile of
[`ci-machinery`](https://github.com/cloudnative-pg/ci-machinery),
which `Taskfile.yml` includes as `ci-machinery-olm`.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) with the `buildx` plugin
- [Task](https://taskfile.dev/installation/)
- [Dagger](https://docs.dagger.io/install/)

Kind, `kubectl`, `operator-sdk` and `opm` run inside the Dagger engine,
so they don't need to be installed.

## Running the tests

```bash
task olm:all ENVIRONMENT=testing
```

This command will, in order:

- Build and push the operator and operand images (`olm:containerize`)
- Generate the bundle, then build and push its image (`olm:bundle`,
  `olm:build`)
- Build the catalog image and the CatalogSource YAML (`olm:catalog`,
  `olm:catalog-source`)
- Validate the bundle with `operator-sdk bundle validate`, including
  the OperatorHub checks (`olm:validate`)
- Run `operator-sdk scorecard` on a Kind cluster (`olm:scorecard`)
- Install OLM on the Kind cluster, install the operator from a catalog
  holding only the bundle, wait for the CSV to succeed and uninstall it
  (`olm:test-catalog`)

The bundle depends on CloudNativePG 1.30.0 or later, declared in
`operator/config/olm-metadata/dependencies.yaml`, so OLM also installs
CloudNativePG from the OperatorHub catalog. Uninstalling Klio leaves
CloudNativePG installed; `olm:prune` removes it with the Kind cluster.

Each of these tasks can also be run on its own.

## Registry

The images go to the registry in `OLM_REGISTRY`, which defaults to:

- `ghcr.io/<owner>` in GitHub Actions
- the local `ci-machinery` registry, `registry.cnpg:5000`, otherwise

`olm:setup-infra` starts the local registry, together with the Dagger
engine and the Docker builder the tasks use. From the host, the
registry is reachable on port 5005 (`OLM_REGISTRY_HOST_PORT`).

To use an external registry, log in to it and set `OLM_REGISTRY`:

```bash
docker login registry.example.com
task olm:all ENVIRONMENT=testing OLM_REGISTRY=registry.example.com/klio
```

If the registry is private, the tasks running in Dagger and the Kind
cluster also need its credentials:

```bash
export PRIVATE_REGISTRY_PASSWORD=<password or token>
task olm:all ENVIRONMENT=testing \
  OLM_REGISTRY=registry.example.com/klio \
  PRIVATE_REGISTRY_HOST=registry.example.com \
  PRIVATE_REGISTRY_USERNAME=<user>
```

## OperatorHub tests

`task olm:fruit-tests ENVIRONMENT=testing` runs the OperatorHub
[operator test suite][opp] (kiwi, lemon and orange) against the
bundle. The test script modifies
the machine it runs on, so the task only runs inside a GitHub Actions
runner.

## CI

The `olm` job in `.github/workflows/ci.yml` runs `task olm:all` after
the `core` and `operator` jobs, which have already pushed the images
to `ghcr.io`, so `olm:containerize` is skipped. The `olm-fruit-tests`
job then runs `task olm:fruit-tests` on its own runner.

## Cleaning up

```bash
# Uninstall OLM from the Kind cluster
task olm:olm-teardown

# Remove the generated files, the Kind cluster, the local registry, the
# Docker builder, the Dagger engine and the Docker network
task olm:prune
```

`olm:prune` removes the `ci-machinery` infrastructure, which other
projects using the `ci-machinery` defaults share.

[opp]: https://k8s-operatorhub.github.io/community-operators/operator-test-suite/
