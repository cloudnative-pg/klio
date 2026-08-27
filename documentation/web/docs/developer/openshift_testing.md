---
sidebar_position: 4
---

# OpenShift

## Security Context Constraints

OpenShift enforces
[Security Context Constraints](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/authentication_and_authorization/managing-pod-security-policies)
(SCCs) to control the actions that pods can perform and the resources
they can access. Under the default **restricted** SCC, containers are
not allowed to run as a fixed user ID. Instead, OpenShift assigns an
arbitrary UID from a range that is unique to each namespace.

As described in the OpenShift
[image creation guidelines](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/images/creating-images#use-uid_create-images):

> By default, OpenShift Container Platform runs containers using an
> arbitrarily assigned user ID. This provides additional security
> against processes escaping the container due to a container engine
> vulnerability and thereby achieving escalated permissions on the
> host node.

This means that specifying a fixed `runAsUser`, `runAsGroup`, or
`fsGroup` in a pod's security context is rejected by the restricted
SCC unless the value falls within the namespace's allocated range.

### How Klio handles this

The Klio Operator detects OpenShift at startup by querying the
Kubernetes discovery API for the `securitycontextconstraints`
resource in the `security.openshift.io/v1` API group.

When running on OpenShift:

- **Server pods**: The operator omits `runAsUser`, `runAsGroup`,
  and `fsGroup` from the pod security context, allowing the
  restricted SCC to assign a UID from the namespace's range.
- **Plugin sidecar containers**: The operator omits `runAsUser`
  and `runAsGroup` from the container security context for the
  same reason.

On vanilla Kubernetes, the operator continues to set explicit
UIDs (1000 for server pods, 26 for plugin sidecars) as before.

No user configuration is required. The detection is automatic and
logged at startup:

```
INFO  setup  Cluster capabilities detected  {"haveSecurityContextConstraints": true}
```

## Testing on OpenShift with OLM

This guide explains how to install a test build of the Klio operator
on an OpenShift cluster using OLM (Operator Lifecycle Manager).

:::note
This procedure is for development and testing only. It requires
a catalog image built from the branch under test.
:::

The bundle declares OpenShift 4.14 as the minimum supported version,
through the `com.redhat.openshift.versions` annotation. The value comes
from the `OPENSHIFT_VERSIONS` variable in `Taskfile.yml`, which
`olm:bundle` writes into both `bundle/metadata/annotations.yaml` and
the labels of `bundle.Dockerfile`.

## Prerequisites

- An OpenShift cluster with `oc` CLI configured
- Cluster admin privileges
- cert-manager installed in the cluster (for TLS certificate
  creation)
- A catalog image built with `task olm:catalog ENVIRONMENT=testing`

The Klio operator, bundle, catalog and operand images are public
on `ghcr.io`, so no pull secret is needed.

## 1. Apply the CatalogSource

A CI run creates a catalog image, tagged with the branch name or the PR number:

Examples:
```
ghcr.io/cloudnative-pg/klio-operator-testing:main-catalog
ghcr.io/cloudnative-pg/klio-operator-testing:pr-1325-catalog
```

Create an `openshift_catalogsource.yaml` file pointing to the
catalog image built from your branch:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: klio-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: <catalog-image>
```

Then apply it:

```bash
oc apply -f openshift_catalogsource.yaml
```

Wait for the catalog pod to become ready:

```bash
oc get pods -n openshift-marketplace -w | grep klio
```

## 2. Subscribe to the operator

Create an `openshift_subscription.yaml` file:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: klio-operator
  namespace: openshift-operators
spec:
  channel: stable-v0
  name: klio-operator
  source: klio-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
```

Apply it:

```bash
oc apply -f openshift_subscription.yaml
```

OLM will create an `InstallPlan` and deploy the operator into
the `openshift-operators` namespace.

:::note
The Klio sidecar (operand) image is baked into the operator
Deployment by the bundle, as both the `SIDECAR_IMAGE` and the
`RELATED_IMAGE_SIDECAR` environment variables; the operator prefers
the latter, and only the OLM bundle sets it. `RELATED_IMAGE_SIDECAR`
is the name operator-sdk expects, so that `--use-image-digests`
pins the operand to a digest and copies it into the CSV's
`relatedImages`, which is what disconnected installs mirror. Digest
pinning only happens in CI, where the images live on `ghcr.io`; a
local build leaves both variables on a tag. The images use the same
registry and tag as the operator image by default, with the `klio`
repository instead of `klio-operator`. The Subscription can override
either variable.
:::

## 3. Create TLS certificates

The Klio plugin requires two TLS secrets to establish mutual
TLS with the CNPG operator:

- `klio-plugin-server-tls` — presented by the plugin gRPC server
- `klio-plugin-client-tls` — used by CNPG to authenticate to
  the plugin

They can be created manually, but the recommended method is to use cert-manager to
automatically generate and manage the certificates.

Create a file `openshift_certificates.yaml` with the following
content, adjusting the `namespace` if the operator was installed
outside of `openshift-operators`:

```yaml
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: klio-operator-selfsigned-issuer
  namespace: openshift-operators
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: klio-plugin-server
  namespace: openshift-operators
spec:
  secretName: klio-plugin-server-tls
  dnsNames:
    - klio-operator-plugin
  usages:
    - server auth
  issuerRef:
    name: klio-operator-selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
  duration: 2160h
  renewBefore: 360h
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: klio-plugin-client
  namespace: openshift-operators
spec:
  secretName: klio-plugin-client-tls
  commonName: klio-plugin-client
  usages:
    - client auth
  issuerRef:
    name: klio-operator-selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
  duration: 2160h
  renewBefore: 360h
```

Apply it:

```bash
oc apply -f openshift_certificates.yaml
```

Verify that the secrets have been created by cert-manager:

```bash
oc get secrets -n openshift-operators \
  klio-plugin-server-tls klio-plugin-client-tls
```

## 4. Verify the installation

The installation is complete when the CSV reaches the
`Succeeded` phase. You can check its status with:

```bash
oc get csv -n openshift-operators -w
```

## Red Hat certification

Klio's operator image and OLM bundle are validated against Red Hat's
[Preflight](https://github.com/redhat-openshift-ecosystem/openshift-preflight)
certification policies. Two checks cover the two artifacts:

- **`check container`** — static policy checks on the operator image
  (labels, layers, license, base image). It needs no cluster and runs
  in the Dagger engine on every PR via `task olm:preflight-container`.
  It is run against the UBI variant of the operator image, since the
  base-image policy only accepts a Red Hat UBI base.
- **`check operator`** — installs the bundle through OLM into a live
  OpenShift cluster and verifies it is deployable. Because it needs a
  real OpenShift cluster (OLM and Security Context Constraints), it runs
  via `task olm:preflight-operator`, against the CRC cluster the
  OpenShift E2E job starts or against a local CRC. The bundle and
  catalog images are multi-arch (`linux/amd64` and `linux/arm64`), so
  the check runs natively on either architecture — including CRC on an
  Apple Silicon Mac.

:::note

The operator image is built in two variants: a distroless Debian one,
which is the default and carries the plain tag, and a Red Hat UBI one,
tagged with a `-ubi9` suffix. Certification concerns the UBI variant
only: `check container` runs against it, and the OLM bundle references
it, so `check operator` exercises it too. The operand (`klio`) image is
Debian based and is not part of the operator certification.

:::

### Run `check operator` against CRC

Point `EXTERNAL_KUBECONFIG` at an OpenShift (CRC) cluster and run the
task. The bundle and catalog (index) images must already be published
for the build under test — build them first with
`task olm:catalog ENVIRONMENT=testing` if needed; the certification runs
against the existing bundle rather than rebuilding it.

```bash
export EXTERNAL_KUBECONFIG=/path/to/crc/kubeconfig
task olm:preflight-operator ENVIRONMENT=testing
```

preflight installs the operator through OLM, runs the operator policy,
and the Dagger `preflight` module evaluates the pass/fail verdict
in-engine. Raw artifacts are written to `operator/preflight-artifacts/`.

The task never contacts Red Hat: the checks are always evaluated
locally.

## Publishing the bundle and catalog

`olm:publish` is the release counterpart of `olm:all`: it builds and
pushes the production bundle and catalog images (no `-testing` suffix)
for the tag being released, and leaves `operator/bundle/` on disk. It
refuses to run outside a tagged CI checkout, and refuses to run if the
CSV version derived from `git describe` does not match the tag.

The `olm-bundle` job in `.github/workflows/release-publish.yml` runs
it, attaches the resulting `CatalogSource` manifest to the GitHub
release, and hands `operator/bundle/` to the `publish-bundle` job,
which commits it to `klio/bundles/<version>` in the
[artifacts repository](https://github.com/cloudnative-pg/artifacts).
Both jobs only run for the release GitHub marks as the latest one,
which excludes drafts and prereleases, so a release candidate never
reaches the `stable-v0` channel.

Each release publishes its own catalog image, tagged with the release
version, and that catalog carries exactly one bundle. The upgrade edge
back to the installed version comes from the `olm.skipRange` annotation
`olm:manifest` writes into the CSV, which spans every release from
`OLM_FIRST_BUNDLE_VERSION` up to (but excluding) the one being built.
Raise that variable only to declare a version range unupgradable, and
never lower it.
