---
sidebar_position: 93
---

# Security

This page covers how to report a vulnerability, the security controls
Klio applies, and how to check that the artifacts you deploy were built
and published by the Klio project.

## Vulnerability Reporting

:::important
Please do not report security vulnerabilities through public GitHub issues.
:::

Klio follows a coordinated disclosure model. Report vulnerabilities
privately from the
[Security tab](https://github.com/cloudnative-pg/klio/security) of the
Klio repository, which has *"Private vulnerability reporting"* enabled.

## Security Controls

You configure these controls on the `Server`, the `PluginConfiguration`
or the Helm chart.

| Control | Summary | Details |
| ------- | ------- | ------- |
| Encryption at rest | Base backups and WAL files are encrypted with the same key, which is stored Age-encrypted | [Encryption](klio_server.md#encryption), [Age Encryption](klio_server.md#age-encryption) |
| Encryption in transit | Base backup traffic and WAL streaming both use TLS | [Encryption in Transit](klio_server.md#encryption-in-transit) |
| Client authentication | mTLS: the client certificate's Common Name decides which cluster's backups and WAL files the client can access | [Authentication](klio_server.md#authentication) |
| Server verification | The plugin checks the server certificate against a CA bundle | [TLS configuration](plugin_configuration.md#tls-configuration) |
| Credential rotation | Most credentials are read again when needed, without a restart | [Rotating Credentials](klio_server.md#rotating-credentials) |
| Network exposure | The server listens on a fixed set of ports, which you can use in a NetworkPolicy | [Network Ports](klio_server.md#network-ports) |
| Operator pod | By default, the operator runs as non-root, with the `RuntimeDefault` seccomp profile, no privilege escalation and all capabilities dropped | [Configuration Reference](helm_chart.mdx#configuration-reference) |

## Release Integrity and Supply Chain

Each Klio release publishes these artifacts:

| Artifact                   | Reference                                    | Tag example    |
| -------------------------- | -------------------------------------------- | -------------- |
| Klio (server and sidecar)  | `ghcr.io/cloudnative-pg/klio`                | `v0.0.21`      |
| Operator (distroless)      | `ghcr.io/cloudnative-pg/klio-operator`       | `v0.0.21`      |
| Operator (UBI 9)           | `ghcr.io/cloudnative-pg/klio-operator`       | `v0.0.21-ubi9` |
| Helm chart                 | `ghcr.io/cloudnative-pg/klio-operator-chart` | `0.0.21`       |
| `klio` binary and packages | [GitHub releases](https://github.com/cloudnative-pg/klio/releases) | `v0.0.21` |

The images are multi-architecture indexes for `linux/amd64` and
`linux/arm64`. The binaries come as `.tar.gz` archives and as `.deb` and
`.rpm` packages for the same two architectures.

The release workflow adds the following to each artifact:

| Artifact     | cosign signature | SLSA build provenance | BuildKit SBOM and provenance |
| ------------ | ---------------- | --------------------- | ---------------------------- |
| Images       | yes              | yes                   | yes                          |
| Helm chart   | yes              | yes                   | no                           |
| Binaries     | no               | yes                   | no                           |

### Prerequisites

- [cosign](https://github.com/sigstore/cosign) v3 or later, to verify the
  signatures
- the [GitHub CLI](https://cli.github.com/) (`gh`), logged in with
  `gh auth login`, to verify the SLSA build provenance
- [Docker Buildx](https://docs.docker.com/build/install-buildx/), included
  in Docker Desktop and recent Docker versions, to resolve digests and read
  the BuildKit attestations
- [jq](https://jqlang.org/), to filter the JSON output in the examples

### Verifying the Signatures

The images and the Helm chart are signed with
[cosign](https://github.com/sigstore/cosign), a signature tool from
[Sigstore](https://www.sigstore.dev/). The signing runs in GitHub Actions
and uses
[short-lived tokens issued through OpenID Connect](https://docs.github.com/en/actions/security-for-github-actions/security-hardening-your-deployments/about-security-hardening-with-openid-connect)
instead of a long-lived key.

A valid signature must match two values:

- the OIDC issuer, `https://token.actions.githubusercontent.com`
- the signing identity, the `release-publish.yml` workflow of the
  [Klio repository](https://github.com/cloudnative-pg/klio/) running on a
  release tag (`refs/tags/v...`)

A signature is bound to a digest, while a tag can be moved to a different
image. Resolve the digest of the tag first, then verify the image by
digest.

To verify the Klio image:

<!-- x-release-please-start-version -->
```sh
DIGEST=$(docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio:v0.0.21 \
  --format '{{ .Manifest.Digest }}')

cosign verify "ghcr.io/cloudnative-pg/klio@${DIGEST}" \
  --certificate-identity-regexp="^https://github\.com/cloudnative-pg/klio/\.github/workflows/release-publish\.yml@refs/tags/v" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```
<!-- x-release-please-end -->

On success, `cosign` prints the checks it ran to the standard error,
followed by the signature payload in JSON on the standard output:

```console
Verification for ghcr.io/cloudnative-pg/klio@sha256:<DIGEST> --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - The code-signing certificate was verified using trusted certificate authority certificates
```

If the image is not signed, or was signed by a different identity,
`cosign` exits with a non-zero status and prints an error.

To verify the distroless operator image:

<!-- x-release-please-start-version -->
```sh
DIGEST=$(docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio-operator:v0.0.21 \
  --format '{{ .Manifest.Digest }}')

cosign verify "ghcr.io/cloudnative-pg/klio-operator@${DIGEST}" \
  --certificate-identity-regexp="^https://github\.com/cloudnative-pg/klio/\.github/workflows/release-publish\.yml@refs/tags/v" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```
<!-- x-release-please-end -->

To verify the UBI 9 operator image:

<!-- x-release-please-start-version -->
```sh
DIGEST=$(docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio-operator:v0.0.21-ubi9 \
  --format '{{ .Manifest.Digest }}')

cosign verify "ghcr.io/cloudnative-pg/klio-operator@${DIGEST}" \
  --certificate-identity-regexp="^https://github\.com/cloudnative-pg/klio/\.github/workflows/release-publish\.yml@refs/tags/v" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```
<!-- x-release-please-end -->

To verify the Helm chart, whose tags have no `v` prefix:

<!-- x-release-please-start-version -->
```sh
DIGEST=$(docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio-operator-chart:0.0.21 \
  --format '{{ .Manifest.Digest }}')

cosign verify "ghcr.io/cloudnative-pg/klio-operator-chart@${DIGEST}" \
  --certificate-identity-regexp="^https://github\.com/cloudnative-pg/klio/\.github/workflows/release-publish\.yml@refs/tags/v" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```
<!-- x-release-please-end -->

The release workflow signs each image index together with its
per-platform images, so the same command also works on the digest of a
single platform, for example the one a node pulled.

### Verifying the SLSA Build Provenance

The images, the Helm chart and the binaries come with a
[SLSA](https://slsa.dev/) build provenance attestation (Build Level 2),
generated with
[GitHub artifact attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations).
It records the repository, the workflow and the commit that produced the
artifact, and is signed with the same keyless identity as the cosign
signatures. Verify it with
[`gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify).

The command checks that the attestation was issued by the
`release-publish.yml` workflow of the Klio repository, for the given
release tag. On success it prints `✓ Verification succeeded!` and exits
with status 0.

To verify the Klio image, pass its digest:

<!-- x-release-please-start-version -->
```sh
DIGEST=$(docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio:v0.0.21 \
  --format '{{ .Manifest.Digest }}')

gh attestation verify "oci://ghcr.io/cloudnative-pg/klio@${DIGEST}" \
  --repo cloudnative-pg/klio \
  --signer-workflow cloudnative-pg/klio/.github/workflows/release-publish.yml \
  --source-ref refs/tags/v0.0.21
```
<!-- x-release-please-end -->

For the operator images and the Helm chart, use the same commands with
the references from the table above. For example, for the Helm chart:

<!-- x-release-please-start-version -->
```sh
DIGEST=$(docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio-operator-chart:0.0.21 \
  --format '{{ .Manifest.Digest }}')

gh attestation verify "oci://ghcr.io/cloudnative-pg/klio-operator-chart@${DIGEST}" \
  --repo cloudnative-pg/klio \
  --signer-workflow cloudnative-pg/klio/.github/workflows/release-publish.yml \
  --source-ref refs/tags/v0.0.21
```
<!-- x-release-please-end -->

To verify a binary archive or package, download it from the
[GitHub release](https://github.com/cloudnative-pg/klio/releases) and
pass the file. For example, for the `linux/amd64` archive:

<!-- x-release-please-start-version -->
```sh
gh release download v0.0.21 \
  --repo cloudnative-pg/klio \
  --pattern klio_Linux_x86_64.tar.gz

gh attestation verify klio_Linux_x86_64.tar.gz \
  --repo cloudnative-pg/klio \
  --signer-workflow cloudnative-pg/klio/.github/workflows/release-publish.yml \
  --source-ref refs/tags/v0.0.21
```
<!-- x-release-please-end -->

### Inspecting the BuildKit Attestations

BuildKit generates two
[build attestations](https://docs.docker.com/build/metadata/attestations/)
for each platform of each image, and the registry stores them next to the
image:

- A [Software Bill of Materials
  (SBOM)](https://docs.docker.com/build/metadata/attestations/sbom/) that
  lists the packages in the image, in the
  [in-toto SPDX format](https://github.com/in-toto/attestation/blob/main/spec/predicates/spdx.md).
- A [build provenance
  attestation](https://docs.docker.com/build/metadata/attestations/slsa-provenance/)
  that records how the image was built: the source repository and commit,
  the base images and the build arguments, in the
  [SLSA provenance format](https://slsa.dev/provenance/).

These attestations describe the image content. To check who built the
image, use the [signatures](#verifying-the-signatures) and the
[SLSA build provenance](#verifying-the-slsa-build-provenance) above.

The examples below read the attestations of the Klio image for
`linux/amd64`. For another image, change the reference as in the table
above. For the other platform, replace `linux/amd64` with `linux/arm64`.

#### Software Bill of Materials

To print the full SBOM in SPDX JSON format:

<!-- x-release-please-start-version -->
```sh
docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio:v0.0.21 \
  --format '{{ json (index .SBOM "linux/amd64").SPDX }}'
```
<!-- x-release-please-end -->

To list only the name and version of each package:

<!-- x-release-please-start-version -->
```sh
docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio:v0.0.21 \
  --format '{{ json (index .SBOM "linux/amd64").SPDX }}' |
  jq -r '.packages[] | "\(.name) \(.versionInfo)"'
```
<!-- x-release-please-end -->

#### Build Provenance

To print the full provenance in SLSA JSON format:

<!-- x-release-please-start-version -->
```sh
docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio:v0.0.21 \
  --format '{{ json (index .Provenance "linux/amd64").SLSA }}'
```
<!-- x-release-please-end -->

To print only the source repository and the commit the image was built
from:

<!-- x-release-please-start-version -->
```sh
docker buildx imagetools inspect ghcr.io/cloudnative-pg/klio:v0.0.21 \
  --format '{{ json (index .Provenance "linux/amd64").SLSA }}' |
  jq '.runDetails.metadata.buildkit_metadata.vcs'
```
<!-- x-release-please-end -->

The output looks like this:

```json
{
  "localdir:context": "core",
  "localdir:dockerfile": "core",
  "revision": "<COMMIT>",
  "source": "https://github.com/cloudnative-pg/klio"
}
```

`revision` is the commit of the release tag in the Klio repository.
