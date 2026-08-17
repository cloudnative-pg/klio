---
sidebar_position: 2
---

# Quickstart

This guide walks you through a minimal Klio installation: the Klio
operator, one Klio server using Tier 1 (local) storage, and a
PostgreSQL cluster streaming its WAL files and base backups to it.

Follow the steps in order — each one builds on the previous.

:::info
This is a deliberately minimal setup. It uses self-signed
certificates, a single PostgreSQL instance and Tier 1 storage only.
See [Next steps](#next-steps) for the reference documentation covering
object storage, retention, sizing and production concerns.
:::

## Before you start

You need:

- A Kubernetes cluster and `kubectl` configured to access it
- A default storage class (check with `kubectl get storageclass`)
- **Helm** — see the
  [Helm installation guide](https://helm.sh/docs/intro/install/)
- **CloudNativePG** already installed — see the
  [CloudNativePG installation guide](https://cloudnative-pg.io/documentation/current/installation_upgrade/)
- **cert-manager** already installed — see the
  [cert-manager installation guide](https://cert-manager.io/docs/installation/)
- The [Age](https://github.com/FiloSottile/age) CLI, to create the
  backup encryption key

This guide installs the operator into the `cnpg-system` namespace and
creates everything else in `default`.

:::warning Namespace requirement
The Klio operator must run in the **same namespace as the
CloudNativePG operator**. If you installed CloudNativePG somewhere
other than `cnpg-system`, replace `cnpg-system` throughout this guide
with your namespace.
:::

## Step 1: Install the Klio operator

Create a `values.yaml` file. The chart expects the Prometheus Operator
by default, so disable it unless you have it installed:

```yaml
# The Prometheus Operator is not needed for this guide
prometheus:
  enable: false
```

Install the chart:

<!-- x-release-please-start-version -->
```sh
helm install klio-operator \
  oci://ghcr.io/cloudnative-pg/klio-operator-chart \
  --version 0.0.20 \
  --namespace cnpg-system \
  -f values.yaml
```
<!-- x-release-please-end -->

Keep `values.yaml` — you will reuse it when
[upgrading](helm_chart.mdx#upgrades).

Check that the operator is running and its custom resource
definitions were created:

```sh
kubectl get pods -n cnpg-system -l app.kubernetes.io/name=klio
kubectl get crds | grep klio.cnpg.io
```

You should see the operator pod in a `Running` state, along with the
`servers.klio.cnpg.io` and `pluginconfigurations.klio.cnpg.io` CRDs.

## Step 2: Create the encryption key

Klio encrypts your backups at rest. The encryption key is itself
protected with [Age](https://github.com/FiloSottile/age), so that the
credential can be rotated later without touching the backup
repository.

Generate an Age key pair:

```sh
age-keygen -o identity.txt
```

The command prints the corresponding public key:

```
Public key: age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p
```

Generate a random encryption key and encrypt it with that public key:

```sh
openssl rand -hex 32 | age -e \
    -i identity.txt \
    -o encryption-key.age
```

Store both files as Kubernetes secrets:

```sh
kubectl create secret generic klio-encryption-key-age \
    --from-file=encryption-key.age
kubectl create secret generic klio-age-identity \
    --from-file=identity.txt
```

:::danger Back up your keys
Keep `identity.txt` and `encryption-key.age` somewhere safe outside
the cluster. Losing them means permanent, unrecoverable loss of access
to every backup. There is no key recovery mechanism.
:::

## Step 3: Create the certificates

Klio secures all traffic with TLS and authenticates clients with
mutual TLS. This step creates four certificates with cert-manager:

- a **CA**, used to sign and verify client certificates
- a **server certificate**, presented by the Klio server
- a **client certificate**, presented by the PostgreSQL instances

Save the following as `klio-certificates.yaml`:

```yaml
---
# Self-signed issuer, used to bootstrap the CA and the server
# certificate. Trust is established through configuration, so a
# self-signed root is not a security problem here.
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: default
spec:
  selfSigned: {}
---
# The CA that signs client certificates
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: klio-server-ca
  namespace: default
spec:
  commonName: klio-server-ca
  secretName: klio-server-ca
  duration: 2160h # 90d
  renewBefore: 360h # 15d
  isCA: true
  usages:
    - cert sign
  issuerRef:
    name: selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
---
# An issuer backed by the CA above, used for client certificates
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: klio-server-ca-issuer
  namespace: default
spec:
  ca:
    secretName: klio-server-ca
---
# The certificate presented by the Klio server
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: klio-server-tls
  namespace: default
spec:
  secretName: klio-server-tls
  commonName: klio-server
  dnsNames:
    - klio-server
    - klio-server.default
    - klio-server.default.svc
    - klio-server.default.svc.cluster.local
  duration: 2160h # 90d
  renewBefore: 360h # 15d
  isCA: false
  usages:
    - server auth
  issuerRef:
    name: selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
---
# The certificate presented by the PostgreSQL instances.
# The Common Name has the form "<user>@<clusterName>": the host part
# MUST match the clusterName in the PluginConfiguration below.
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: cluster-example-klio-user
  namespace: default
spec:
  secretName: cluster-example-klio-user
  commonName: klio@cluster-example
  duration: 2160h # 90d
  renewBefore: 360h # 15d
  isCA: false
  usages:
    - client auth
  issuerRef:
    name: klio-server-ca-issuer
    kind: Issuer
    group: cert-manager.io
```

Apply it and wait for cert-manager to issue everything:

```sh
kubectl apply -f klio-certificates.yaml
kubectl wait --for=condition=Ready certificate --all --timeout=120s
```

:::info
For production, use certificates signed by your organization's
Certificate Authority instead of a self-signed root. See
[Authentication](klio_server.md#authentication) for the rules that
client certificates must satisfy.
:::

## Step 4: Create the Klio server

The `Server` resource creates a StatefulSet running the Klio server,
along with the persistent volumes holding your backups.

Save the following as `klio-server.yaml`:

<!-- x-release-please-start-version -->
```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: Server
metadata:
  name: klio-server
  namespace: default
spec:
  image: ghcr.io/cloudnative-pg/klio:v0.0.20

  # TLS certificate presented to clients
  tlsSecretName: klio-server-tls
  # CA used to verify client certificates
  caSecretName: klio-server-ca

  tier1:
    # Kopia cache. The default Kopia cache is 5 GB of content plus
    # 5 GB of metadata, so leave some headroom.
    cache:
      pvcTemplate:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 10Gi
    # Base backups and the WAL archive
    data:
      pvcTemplate:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 20Gi
    encryptionKeyFile:
      fileReference:
        volume:
          secret:
            secretName: klio-encryption-key-age
        path: encryption-key.age
    identityFile:
      fileReference:
        volume:
          secret:
            secretName: klio-age-identity
        path: identity.txt

  # Work queue, required whenever tier1 is configured
  queue:
    pvcTemplate:
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 50Mi
```
<!-- x-release-please-end -->

Apply it:

```sh
kubectl apply -f klio-server.yaml
```

The volumes above use the default storage class. Set
`storageClassName` in each `pvcTemplate` to choose a different one,
and see [Storage Requirements](klio_server.md#storage-requirements)
for how to size them for real workloads.

Wait for the server pod to come up:

```sh
kubectl get pods -l klio.cnpg.io/klio-server=klio-server -w
```

The server initializes the backup repository on first start, so give
it a moment to reach `Running`. If it does not, check the logs:

```sh
kubectl logs -l klio.cnpg.io/klio-server=klio-server -f
```

## Step 5: Configure the PostgreSQL cluster

Two resources are needed: a `PluginConfiguration` telling the Klio
plugin how to reach the server, and a CloudNativePG `Cluster`
referencing it.

Save the following as `cluster-example.yaml`:

```yaml
---
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: klio-plugin-config
  namespace: default
spec:
  # The Klio server Service is named after the Server resource
  serverAddress: klio-server.default
  clientSecretName: cluster-example-klio-user
  serverSecretName: klio-server-tls
  # Must match the host part of the client certificate Common Name
  clusterName: cluster-example
---
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: cluster-example
  namespace: default
spec:
  instances: 1

  postgresql:
    pg_hba:
      # Required so that Klio can stream WAL files locally
      - local replication all peer map=local

  plugins:
    - name: klio.cnpg.io
      enabled: true
      parameters:
        pluginConfigurationRef: klio-plugin-config

  storage:
    size: 1Gi
```

Apply it:

```sh
kubectl apply -f cluster-example.yaml
```

:::warning Order matters
The `PluginConfiguration` must exist before the `Cluster`. If it does
not, the Klio plugin gates reconciliation and the cluster waits
without progressing until you create it.
:::

A few details worth noting:

- The `pg_hba` entry is what allows Klio to open a local replication
  connection and stream WAL.
- Do **not** set `isWALArchiver`. Klio streams WAL files directly to
  the server rather than going through PostgreSQL's `archive_command`.
- `clusterName` in the `PluginConfiguration` must match the host part
  of the client certificate Common Name (`klio@cluster-example`
  above). Klio refuses the connection on a mismatch.
- Reusing the `Cluster` name as `clusterName`, as this guide does, is
  only a convention — the two are independent. See
  [Cluster name override](plugin_configuration.md#cluster-name-override).

## Step 6: Verify the setup

Check that the Klio server is ready:

```sh
kubectl get server klio-server
kubectl get pods -l klio.cnpg.io/klio-server=klio-server
```

Check that the PostgreSQL cluster is healthy:

```sh
kubectl get cluster cluster-example
```

Finally, confirm that WAL streaming has started. The plugin creates a
physical replication slot named `klio` on the primary:

```sh
kubectl logs -l klio.cnpg.io/klio-server=klio-server
```

You should see lines with the `Received completed WAL file` message.

## Next steps

- [Take your first backup](backup_and_restore.md) and learn how to
  restore a cluster or perform point-in-time recovery
- [The Klio Server](klio_server.md) — storage tiers, sizing the
  volumes, Tier 2 object storage, read-only servers, encryption and
  authentication
- [The Klio Plugin](plugin_configuration.md) — retention policies,
  Tier 2, WAL prefetch and sidecar customization
- [Architectures & Tiers](concepts/architectures.md) — planning a backup
  strategy
- [Klio Operator Helm Chart](helm_chart.mdx) — the full chart
  configuration reference and the upgrade procedure
- [OpenTelemetry](opentelemetry.md) and
  [Grafana dashboards](grafana-dashboards.md) — monitoring
