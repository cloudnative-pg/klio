---
sidebar_position: 3
---

# Testing the OLM bundle

This guide explains how to build and test the Klio OLM (Operator
Lifecycle Manager) bundle. The tasks come from
[`ci-machinery`](https://github.com/cloudnative-pg/ci-machinery),
which `Taskfile.yml` includes twice, as `ci-machinery-infra` and
`ci-machinery-olm`.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) with the `buildx` plugin
- [Task](https://taskfile.dev/installation/)
- [Dagger](https://docs.dagger.io/install/)

Kind, `kubectl`, `operator-sdk` and `opm` run inside the Dagger engine,
so they don't need to be installed.

## Running the tests

```bash
task olm:test
```

This command will, in order:

- Start a local container registry, `registry.cnpg:5000`
- Build and push the operator and operand images for the host platform
  (`olm:containerize`)
- Render the OLM manifests (`olm:kustomization`), then generate the
  bundle and build and push its image
- Validate the bundle with `operator-sdk bundle validate`, including
  the OperatorHub checks
- Create a Kind cluster with OLM installed, install the operator from a
  catalog holding only the bundle, wait for the CSV to succeed and
  uninstall it
- Run `operator-sdk scorecard` on the Kind cluster
- Run the OperatorHub fruit tests (see below)

Everything runs internally: the registry, the Docker builder, the
Dagger engine and the Kind cluster are created by the task itself, and
removed when it ends, whether it succeeds or fails, together with the
generated manifests, bundle and kubeconfig. Nothing is pushed to an
external registry.

The teardown removes the `ci-machinery` infrastructure, which other
projects using the `ci-machinery` defaults share.

The bundle depends on CloudNativePG 1.30.0 or later, declared in
`operator/config/olm/metadata/dependencies.yaml`, so OLM also installs
CloudNativePG from the OperatorHub catalog.

### Running locally

```bash
task olm:test SKIP_FRUIT_TESTS=true
```

The OperatorHub [operator test suite][opp] (kiwi, lemon and orange)
installs tools on the machine it runs on and changes its
configuration, so it is only meant to run on a disposable GitHub
Actions runner, and refuses to run anywhere else. Outside GitHub
Actions, set `SKIP_FRUIT_TESTS=true`, otherwise the task fails at that
last step.

### Options

- `SKIP_CONTAINERS_BUILD=true` reuses the images already in the
  registry instead of building them.
- `OPERATOR_IMAGE` and `SIDECAR_IMAGE` set the operator and operand
  images the bundle references. They default to
  `registry.cnpg:5000/klio-operator-testing:<version>` and
  `registry.cnpg:5000/klio-testing:<version>`, where `<version>` is
  derived from `git describe`.
- `PLATFORMS` sets the platforms of the bundle and catalog images,
  `linux/<host architecture>` by default.

## CI

The `olm` job in `.github/workflows/ci.yml` runs `task olm:test`,
fruit tests included, whenever the core or the operator module
changes.

[opp]: https://k8s-operatorhub.github.io/community-operators/operator-test-suite/
