---
sidebar_position: 1.5
---

# Quick Start

This guide walks you through deploying a Klio server, configuring a
CloudNativePG cluster to back up to it, taking a backup, and restoring
it — end to end, on a single Tier 1 (local storage) server with
self-signed certificates. It's meant to get you to a working setup
quickly for evaluation purposes.

For production topology decisions, see [Architectures](architectures.md).
For every option skipped here — Helm chart configuration, Tier 2 object
storage, read-only disaster-recovery servers, retention tuning, encryption
key rotation, IAM-based S3 authentication, container customization, and
more — see [Klio Operator Helm Chart](helm_chart.mdx),
[The Klio Server](klio_server.md), [The Klio Plugin](plugin_configuration.md),
and [Backup and Restore](backup_and_restore.md).

## Prerequisites

- A Kubernetes cluster with [CloudNativePG](https://cloudnative-pg.io)
  installed
- `kubectl` configured to access your cluster
- [Helm](https://helm.sh/docs/intro/install/) installed (to install the
  Klio operator)
- [cert-manager](https://cert-manager.io/) installed for certificate
  management
- [Age](https://github.com/FiloSottile/age) CLI installed locally (for
  encrypting the backup encryption key)
- Enough storage resources for the Klio server's data, cache, and queue
  PersistentVolumeClaims

## Install the Klio Operator

Deploy the Klio Operator using its Helm chart, into the same namespace as
CloudNativePG (`cnpg-system` in these examples). This is a cluster-wide
component, separate from the `default` namespace used for the `Server`
and `Cluster` resources created later in this guide.

If you don't have the Prometheus Operator installed, disable the
corresponding chart dependency:

```yaml
# Uncomment if the Prometheus Operator is not installed
# prometheus:
#   enable: false
```

<!-- x-release-please-start-version -->
```sh
helm install klio-operator \
  oci://ghcr.io/cloudnative-pg/klio-operator-chart \
  --version 0.0.17 \
  --namespace cnpg-system \
  -f values.yaml
```
<!-- x-release-please-end -->

Verify the operator is running and the CRDs were created:

```sh
kubectl get pods -n cnpg-system -l app.kubernetes.io/name=klio
kubectl get crds | grep klio.cnpg.io
```

For the full configuration reference, upgrade procedure, and uninstall
instructions, see [Klio Operator Helm Chart](helm_chart.mdx).

## Step 1: Deploy a Klio Server

### 1.1 Create the encryption key

The encryption key protects backup data at rest. Klio uses
[Age](https://github.com/FiloSottile/age) encryption to protect the key
itself, so it can be rotated without touching the Kopia repository.

Generate an Age key pair:

```bash
age-keygen -o identity.txt
# Public key: age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p
```

Generate a random encryption key and encrypt it with the public key:

```bash
openssl rand -hex 32 | age \
    -r age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p \
    -o encryption-key.age
```

Create Kubernetes Secrets for both files:

```bash
kubectl create secret generic my-server-encryption-key \
    --from-file=encryption-key.age
kubectl create secret generic my-server-age-identity \
    --from-file=identity.txt
```

:::tip
Use a strong, randomly generated key. Store it securely — there is no key
recovery mechanism.
:::

### 1.2 Create a CA certificate

The CA is used to authenticate Klio clients (the plugin sidecars) via
mTLS. Using cert-manager, create a self-signed CA:

```yaml
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: default
spec:
  selfSigned: { }
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: server-sample-ca
spec:
  commonName: server-sample-ca
  secretName: server-sample-ca

  duration: 2160h # 90d
  renewBefore: 360h # 15d

  isCA: true
  usages:
    - cert sign

  issuerRef:
    name: selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
```

```bash
kubectl apply -f ca-configuration.yaml
```

### 1.3 Create the server's TLS certificate

```yaml
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: default
spec:
  selfSigned: { }
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-server-cert
  namespace: default
spec:
  secretName: my-server-tls
  commonName: my-server
  dnsNames:
    - my-server
    - my-server.default
    - my-server.default.svc
    - my-server.default.svc.cluster.local
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  isCA: false
  usages:
    - server auth
  issuerRef:
    name: selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
```

```bash
kubectl apply -f tls-certificate.yaml
```

:::info
For production environments, use certificates signed by your
organization's Certificate Authority (CA) or a trusted public CA instead
of self-signed certificates.
:::

### 1.4 Create the Server resource

<!-- x-release-please-start-version -->
```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: Server
metadata:
  name: my-server
  namespace: default
spec:
  # Container image for the Klio server
  image: ghcr.io/cloudnative-pg/klio:v0.0.17
  imagePullPolicy: IfNotPresent

  # TLS configuration
  tlsSecretName: my-server-tls

  # Client authentication configuration
  caSecretName: server-sample-ca

  # tier 1 configuration
  tier1:
    # Cache storage configuration
    cache:
      pvcTemplate:
        storageClassName: standard  # Adjust to your storage class (use 'kubectl get storageclass' to see available options)
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 10Gi  # Adjust based on your needs
    # Data storage pvcTemplate (for backups and WAL)
    data:
      pvcTemplate:
        storageClassName: standard  # Adjust to your storage class (use 'kubectl get storageclass' to see available options)
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 100Gi  # Adjust based on your backup needs
    # Age-encrypted encryption key file
    encryptionKeyFile:
      fileReference:
        volume:
          secret:
            secretName: my-server-encryption-key
        path: encryption-key.age
    # Age identity file for decryption
    identityFile:
      fileReference:
        volume:
          secret:
            secretName: my-server-age-identity
        path: identity.txt

  # Queue storage configuration (for NATS work queue)
  # Required when tier1 is configured
  queue:
    pvcTemplate:
      storageClassName: standard  # Adjust to your storage class
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 50Mi
```
<!-- x-release-please-end -->

```bash
kubectl apply -f klio-server.yaml
```

### 1.5 Verify the server is running

```bash
# Check the Server resource status
kubectl get server my-server -n default

# Check the StatefulSet
kubectl get statefulset my-server-klio -n default

# Check the Pod
kubectl get pods -l klio.cnpg.io/klio-server=my-server -n default

# View logs
kubectl logs -l klio.cnpg.io/klio-server=my-server -n default -f
```

The server should create a StatefulSet with a pod named `my-server-klio-0`.

## Step 2: Configure a Cluster with the Klio Plugin

### 2.1 Create a client certificate

This certificate authenticates the PostgreSQL cluster's Klio sidecar to
the server. The `commonName` must follow the `<user>@<clusterName>`
format, where `<clusterName>` matches the `clusterName` you'll set on the
`PluginConfiguration` below.

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: client-sample-tls
spec:
  secretName: client-sample-tls
  commonName: klio@my-cluster

  duration: 2160h # 90d
  renewBefore: 360h # 15d

  isCA: false
  usages:
    - client auth

  issuerRef:
    name: server-sample-ca
    kind: Issuer
    group: cert-manager.io
```

```bash
kubectl apply -f client-certificate.yaml
```

### 2.2 Create a PluginConfiguration

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: klio-plugin-config
  namespace: default
spec:
  serverAddress: my-server.default
  clientSecretName: client-sample-tls
  serverSecretName: my-server-tls
  clusterName: my-cluster
```

```bash
kubectl apply -f klio-plugin-config.yaml
```

### 2.3 Reference the plugin in your Cluster

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: my-cluster
  namespace: default
spec:
  instances: 3

  postgresql:
    pg_hba:
      - local replication all peer map=local # Allow replication connections locally

  plugins:
    - name: klio.cnpg.io
      enabled: true # Activate the Klio plugin (default)
      parameters:
        pluginConfigurationRef: klio-plugin-config

  storage:
    size: 10Gi
```

```bash
kubectl apply -f my-cluster.yaml
```

The `pg_hba` entry above is required so PostgreSQL accepts the local
replication connection the Klio plugin uses to stream WAL files.

## Step 3: Take a Backup

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Backup
metadata:
  name: my-cluster-backup-20251027
  namespace: default
spec:
  method: plugin
  target: primary
  cluster:
    name: my-cluster
  pluginConfiguration:
    name: klio.cnpg.io
```

```bash
kubectl apply -f backup.yaml
```

See [Monitor Backup Progress](backup_and_restore.md#monitor-backup-progress)
to check on it.

## Step 4: Restore the Backup

Restoring bootstraps a **new** cluster from the backup. First, create a
`PluginConfiguration` pointing back at the original cluster's data:

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: my-restore-config
  namespace: default
spec:
  serverAddress: my-server.default
  clientSecretName: client-sample-tls
  serverSecretName: my-server-tls
  # Must match the name of the original cluster whose backups you are
  # restoring from, not the name of the new cluster being created.
  clusterName: my-cluster
```

```bash
kubectl apply -f restore-config.yaml
```

Then create the restored `Cluster`:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: my-restored-cluster
  namespace: default
spec:
  instances: 3

  # Bootstrap from a Klio backup
  bootstrap:
    recovery:
      source: source
      # OPTIONAL: Specify the backup to restore from; Klio picks the
      # latest one automatically if omitted
      recoveryTarget:
        backupID: my-cluster-backup-YYYYMMDDHHMMSS

  # Reference the Klio plugin configuration
  externalClusters:
    - name: source
      plugin:
        name: klio.cnpg.io
        parameters:
          pluginConfigurationRef: my-restore-config

  storage:
    size: 10Gi
```

```bash
kubectl apply -f restored-cluster.yaml
```

The restored cluster operates independently and will **not** perform its
own backups unless you configure the Klio plugin for backup operations on
it too, as in Step 2.

## Next Steps

- [Architectures](architectures.md) — production topology options,
  including Tier 2 object storage and read-only disaster-recovery servers
- [Managing Storage](managing_storage.md) — sizing and resizing PVCs
- [OpenTelemetry](opentelemetry.md) — monitoring and telemetry
