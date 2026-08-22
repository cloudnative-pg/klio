---
sidebar_position: 6
---

# The Klio Server

The Klio server is a central component of the Klio backup solution. It is
defined as the `Server` custom resource in Kubernetes, which creates a
StatefulSet running the Klio server application.

The Klio server runs as a single `server` container. On startup, it first
initializes the Kopia repository, then starts serving both base backups
(using Kopia) and the incoming stream of PostgreSQL Write-Ahead Logs (WAL).

The base backups and WAL files are stored on a single PersistentVolume attached
to the Klio server pod, in the `/data/base` and `/data/wal` directories,
respectively.

## Storage Tiers

### Tier 1: Local Storage

Tier 1 uses local `PersistentVolumes` for immediate data access.
This is the primary landing zone for backups and WAL files,
providing the fastest recovery times.

### Tier 2: Remote Object Storage

Tier 2 offloads data to object storage for long-term retention and disaster
recovery. When Tier 2 is enabled alongside Tier 1, the server uses a work
queue to manage the asynchronous transfer of data from local storage to object
storage.

Alternatively, you can deploy a **read-only server** with only Tier 2
configured. This is useful for disaster recovery sites that need to restore
from object storage without the overhead of local storage. See the
[Read-Only Mode](#read-only-mode) section for details.

Currently, Klio supports only Amazon S3 and S3-compatible storage providers.
See the [Object Store](#object-store) section for configuration details.

### The Work Queue

When Tier 1 is configured, the Klio Server pods will use a work queue.
The work queue is backed by NATS JetStream with file storage on a separate
`PersistentVolume` mounted at `/queue`.
The queue serves two purposes:

- **Retention policy enforcement**: Tracks which WAL files are in use before
  deletion
- **Tier 2 replication**: When Tier 2 is enabled, manages asynchronous
  transfer to object storage

## Storage Requirements

The Klio Server uses multiple PersistentVolumeClaims (PVCs), each
serving a different purpose. Understanding what each PVC contains helps you
size them appropriately for your environment. For guidance on managing
storage capacity and resizing PVCs, see
[Managing Storage](managing_storage.md).

### Data PVC

The data PVC stores all backup data and WAL archives for Tier 1 storage.

It holds the base backups and the WAL archive of all the servers that are backed
up.

The following factors should be considered when defining the PVC size:

1. WAL file production rate
1. Base backup size
1. Retention policies

### Cache PVCs

The cache PVCs (one for Tier 1 and Tier 2 each) are used by Kopia for its
[caching operations](https://kopia.io/docs/advanced/caching/).
They are used to speed up snapshot operations.

:::warning
Klio is currently limited to use the default cache size when creating a Kopia
repository, 5GB for content and 5GB for metadata.
The cache sizes are not hard limits, as the cache is swept periodically,
so users should have a space buffer to account for this additional space.
This limitation will be removed in a future version.
:::

### Queue PVC

The queue PVC is required when Tier 1 is configured. It stores the NATS
JetStream work queue used for retention policy enforcement and asynchronous
Tier 2 replication.

#### Queue Sizing Guidelines

The queue stores only task metadata (cluster name and WAL filename), not the
actual WAL content. This means queue size depends on the **number of WAL
segments** generated, not the size of your database.

**Sizing formula:**

```
Queue Size = WAL_segments_per_hour × max_backlog_hours × 300 bytes × 2
```

Where:

- **WAL_segments_per_hour**: How many WAL segments your database generates per
  hour (check with `pg_stat_archiver` or monitor WAL production)
- **max_backlog_hours**: Maximum duration the Tier 2 WAL replication backlog
  can grow before the queue fills up and tasks are lost. Backlog builds up
  when Tier 2 replication falls behind Tier 1 ingestion — for example, during
  Tier 2 outages or when object storage uploads are slower than local disk writes.
- **300 bytes**: Approximate storage per WAL task (message + JetStream overhead)
- **2**: Safety factor

**Recommended sizes:**

| Workload | WAL Rate | Recommended Size |
|----------|----------|------------------|
| Low write (OLTP) | ~60 segments/hour | **10 MiB** |
| Medium write | ~120 segments/hour | **25 MiB** |
| High write | ~360 segments/hour | **50 MiB** |
| Very high write | >500 segments/hour | **100 MiB** |

These recommendations assume a 24-hour backlog tolerance and include an
additional **~10x safety margin** beyond the formula result. This margin accounts
for:

- **Burst workloads**: WAL production can spike significantly above average rates
- **Multiple clusters**: A single Klio server may handle several CNPG clusters
- **Low cost of headroom**: Storage is cheap relative to the risk of queue
  overflow, which causes WAL loss

For shorter tolerance windows, you can reduce the queue size proportionally, but
keep the safety margin.

:::tip
Start with 50 MiB as a conservative default. Monitor queue usage with the
`klio admin queue status` command and adjust based on actual WAL production
rates in your environment.
:::

:::note
Large transactions that modify significant amounts of data will automatically
generate multiple WAL segments (PostgreSQL rotates WAL files at ~16 MB by
default). Account for this when estimating your WAL segment rate.
:::

## Setting up a new Klio server

The [Quickstart](quickstart.md) walks through a complete minimal
setup: the encryption key, the certificates, a Tier 1 `Server` and a
PostgreSQL cluster streaming to it. Start there.

The rest of this page is the reference for the `Server` resource — the
storage tiers and how to size them, read-only servers, object storage,
encryption and authentication.

## Read-Only Mode

Klio servers can operate in read-only mode, allowing them to serve backups and
WAL files from Tier 2 object storage without accepting new backup writes. This
is useful for disaster recovery sites, cost-optimized restore-only deployments,
and multi-region architectures.

### When to Use Read-Only Mode

Use read-only mode when you need:

- **Disaster recovery sites**: Deploy in secondary regions to restore from
  shared S3 storage without duplicating backup writes
- **Geographic distribution**: Multiple read-only servers in different regions
  can all restore from a single S3 bucket populated by one primary server
- **Read-only access control**: Prevent accidental backup modifications at
  certain sites

### Configuration

A read-only server requires:

- `mode: read-only` field in the spec
- `tier2` configuration (S3 object storage)
- **No** `tier1` configuration
- **No** `queue` configuration

:::note
The `mode` field is immutable. Once a Server is created, its mode
cannot be changed. To operate in a different mode, you would need
another Klio server with a different mode.
:::

<!-- x-release-please-start-version -->
```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: Server
metadata:
  name: dr-server
  namespace: default
spec:
  # Set mode to read-only
  mode: read-only

  # Container image for the Klio server
  image: ghcr.io/cloudnative-pg/klio:v0.0.21
  imagePullPolicy: IfNotPresent

  # TLS configuration
  tlsSecretName: dr-server-tls

  # Client authentication configuration
  caSecretName: klio-server-ca

  # Tier 2 configuration (required for read-only mode)
  tier2:
    # Cache storage configuration
    cache:
      pvcTemplate:
        storageClassName: standard
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 10Gi  # Only cache needed, no data storage

    # Age-encrypted encryption key file
    encryptionKeyFile:
      fileReference:
        volume:
          secret:
            secretName: dr-server-encryption-key
        path: encryption-key.age
    # Age identity file for decryption
    identityFile:
      fileReference:
        volume:
          secret:
            secretName: dr-server-age-identity
        path: identity.txt

    # S3 access configuration
    # See Object Store section for authentication options
    s3:
      bucketName: klio-backups
      region: us-east-1
      accessKeyId:
        name: s3-credentials
        key: ACCESS_KEY_ID
      secretAccessKey:
        name: s3-credentials
        key: SECRET_ACCESS_KEY
```
<!-- x-release-please-end -->

Apply the read-only server:

```bash
kubectl apply -f dr-server.yaml
```

### Using a Read-Only Server for Recovery

Once deployed, PostgreSQL clusters can use the read-only server as a restore
source through a PluginConfiguration. The server will fetch backups and WAL
files from Tier 2 object storage transparently.

See the Read-Only Server Mode section in the
[Architectures](concepts/architectures.md) documentation for detailed use cases and
architectural patterns.

### Restrictions

In read-only mode, the following operations are **not available**:

- Creating new backups
- Sending WAL files to the server
- Applying retention policies
- Any write operations

## Advanced Configuration

The `.spec.template` field allows you to customize the Klio server's pod
template. You can add additional containers, volumes, or modify existing
settings.

:::warning Advanced Users Only
The `.spec.template` field is primarily designed for advanced configurations.
While powerful, improper modifications can affect server functionality.
Always test changes in a non-production environment first.
:::

:::note
The `containers` field within `.spec.template.spec` is mandatory but will be
merged with the default Klio `server` container. If you do not need to add
containers or modify the default one, you must still include an empty list.
:::

### Node Affinity and Tolerations

To dedicate specific nodes for Klio workloads (e.g., for performance isolation
or to separate backup workloads from application workloads), you can use the
`template` field to define affinity and toleration rules.

```yaml
spec:
  template:
    spec:
      # Mandatory field; merged with default containers
      containers: []
      tolerations:
        # Allow scheduling on nodes tainted for Klio
        - key: node-role.kubernetes.io/klio
          operator: Exists
          effect: NoSchedule
      affinity:
        # Require nodes labeled for Klio
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: node-role.kubernetes.io/klio
                    operator: Exists
```

See [Reserving Nodes for Klio Workloads](concepts/architectures.md#reserving-nodes-for-klio-workloads)
for details on node tainting.

### Monitoring

Refer to the [OpenTelemetry](./opentelemetry.md#klio-server-with-opentelemetry)
documentation for setting up monitoring and telemetry for the Klio server.

## Object Store

Klio uses object storage for Tier 2, providing durable, cost-effective
long-term backup storage. Currently, Klio supports Amazon S3 and S3-compatible
storage providers.

### S3

Tier 2 is configured using the `tier2.s3` field in the Server spec. The
configuration is the same for both AWS S3 and S3-compatible providers.

#### Basic Configuration with Credentials

```yaml
tier2:
  s3:
    bucketName: klio-backups
    region: us-east-1
    accessKeyId:
      name: s3-credentials
      key: ACCESS_KEY_ID
    secretAccessKey:
      name: s3-credentials
      key: SECRET_ACCESS_KEY
```

#### S3-Compatible Storage with Custom Endpoint

For S3-compatible providers, add the `endpoint` field:

```yaml
tier2:
  s3:
    bucketName: klio-backups
    endpoint: https://<endpoint>:<port>
    region: us-east-1  # May be required depending on provider
    accessKeyId:
      name: s3-credentials
      key: ACCESS_KEY_ID
    secretAccessKey:
      name: s3-credentials
      key: SECRET_ACCESS_KEY
```

#### Custom CA Certificates

For providers using self-signed certificates or custom CAs:

```yaml
tier2:
  s3:
    bucketName: klio-backups
    endpoint: https://<endpoint>:<port>
    customCaBundle:
      name: minio-ca-cert
      key: ca.crt
    accessKeyId:
      name: s3-credentials
      key: ACCESS_KEY_ID
    secretAccessKey:
      name: s3-credentials
      key: SECRET_ACCESS_KEY
```

#### AWS IAM Roles (IRSA/Pod Identity)

For AWS EKS clusters, using
[IAM Roles for Service Accounts (IRSA)](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
or [EKS Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html)
is the recommended approach. This provides better security through automatic
credential rotation, reduced secret sprawl, and fine-grained IAM policies.

To use IAM role-based authentication:

1. Create an IAM role with appropriate S3 permissions
1. Create a Kubernetes ServiceAccount with the IAM role annotation (for IRSA)
   or Pod Identity association (for Pod Identity)
1. Reference the ServiceAccount in the Server spec and omit credentials:

```yaml
spec:
  tier2:
    s3:
      bucketName: klio-backups
      region: us-east-1
      # No accessKeyId or secretAccessKey - use IAM role

  template:
    spec:
      serviceAccountName: klio-s3-access
      containers: []  # Mandatory but merged with defaults
```

The AWS SDK will automatically use the pod's IAM role credentials when
`accessKeyId` and `secretAccessKey` are omitted.

## Encryption

Klio implements encryption at rest for both base backups and WAL files to
ensure data security throughout the backup lifecycle.

### Base Backups Encryption

Base backups are encrypted by Kopia using the encryption key
decrypted from the Age-encrypted key file. Kopia handles
encryption transparently.

The encryption key is set during repository initialization and is required
for all subsequent backup and restore operations.

:::warning Critical
Store the encryption key securely. Loss of this key means permanent
loss of access to all backup data. There is no key recovery mechanism.
:::

### WAL Files Encryption

WAL files are encrypted using a master key derivation system with authenticated
encryption. The encryption process works as follows:

1. **Master Key Generation**: A 32-byte master key is derived from the encryption
   key using PBKDF2
1. **Key Enveloping**: The master key itself is encrypted using AES-256-GCM
   with a password-derived encryption key to protect the key at rest
1. **Per-File Encryption**: Each WAL file is compressed and then encrypted using
   the master key with authenticated encryption before being stored

WAL files are first compressed using Snappy S2 compression,
then encrypted to ensure both space efficiency and security.

The same encryption key used for base backups encrypts the WAL files,
ensuring a unified security model across all backup artifacts.

### Encryption Credential Rotation

The underlying encryption key (used by Kopia and the WAL keychain)
cannot be changed once set. However, you can rotate the Age identity
without touching the encryption key or the repository:

1. Generate a new Age key pair
1. Re-encrypt the encryption key file with the new public key
1. Deploy the new identity file and re-encrypted key file

This rotation only changes how the encryption key is protected,
not the key itself. See [Rotating Age Credentials](#rotating-age-credentials)
for step-by-step instructions.

:::tip
Choose a strong encryption key from the start. Use a password
manager or key management system to generate and store a
cryptographically secure key (recommended: 32+ random characters).
:::

### Encryption in Transit

In addition to encryption at rest, Klio protects both base backups and WAL files
during transmission using TLS (Transport Layer Security).

All communication between a Klio client and the Klio server is secured
with TLS:

- **Base Backup Traffic**: Kopia client connections to the base backup server
  are encrypted using TLS, protecting backup data as it transfers to the Klio
  server
- **WAL Streaming**: PostgreSQL instances streaming WAL files to the Klio server
  use gRPC over TLS, ensuring WAL data is encrypted during transmission

The TLS certificate is configured via the `.spec.tlsSecretName` field in the
Server resource, which references a Kubernetes secret containing the TLS
certificate and private key. This provides end-to-end encryption, ensuring that
backup data is protected both at rest and in transit.

### Age Encryption

[Age](https://github.com/FiloSottile/age) is a modern file
encryption tool that Klio supports for protecting the encryption
key. Instead of storing the plaintext encryption key in a
Kubernetes Secret, you encrypt it with an Age public key and
provide the corresponding Age identity (private key) to Klio.

This enables:

- **Credential rotation** without touching the Kopia repository
  or WAL data.
- **Multiple recipients** for disaster recovery or team access.
- **Offline operations** — re-encryption can be done with the
  standard `age` CLI.

#### Referencing the key files

To generate the Age key pair and the encrypted encryption key, see
[Create the encryption key](quickstart.md#step-2-create-the-encryption-key).

Once the Secrets exist, reference them in the Server spec:

```yaml
tier1:
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
```

The same configuration applies to `tier2`.

:::note
Only standard Age identities (X25519 keys) are supported.
Age plugins (e.g., `age-plugin-yubikey`) are not supported
directly, but you can encrypt the key file to both a
plugin-based recipient and a standard X25519 recipient.
:::

#### Using External Secret Managers

The `encryptionKeyFile` and `identityFile` fields accept any
Kubernetes `VolumeSource`, not just Secrets. This enables
integration with external secret management systems:

```yaml
tier1:
  encryptionKeyFile:
    fileReference:
      volume:
        csi:
          driver: secrets-store.csi.k8s.io
          readOnly: true
          volumeAttributes:
            secretProviderClass: klio-aws-secrets
      path: encryption-key.age
```

#### Rotating Age Credentials

To rotate the Age identity without touching the encryption key
or the repository:

1. Generate a new Age key pair:

```bash
age-keygen -o new-identity.txt
```

2. Re-encrypt the key file with the new public key:

```bash
age -d -i identity.txt encryption-key.age | \
    age -r <new-public-key> -o encryption-key-new.age
```

3. Update the Kubernetes Secrets:

```bash
kubectl create secret generic klio-encryption-key-age \
    --from-file=encryption-key.age=encryption-key-new.age \
    --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic klio-age-identity \
    --from-file=identity.txt=new-identity.txt \
    --dry-run=client -o yaml | kubectl apply -f -
```

4. Restart the Klio server pod to pick up the new files.

5. Securely delete the old identity and plaintext files.

## Authentication

Klio uses mTLS Authentication for securing access to both the base backup server
and the WAL streaming server. Authentication is handled by verifying the client
certificates against the CA certificate which has been created when configuring
the Klio server.

### Creating a client-side certificate

A client certificate is accepted by the Klio server when it satisfies
all of the following:

- It is signed by the CA whose secret is referenced by
  `.spec.caSecretName` on the `Server`. In practice this means signing
  it with a cert-manager `Issuer` backed by that CA secret.
- It carries the `client auth` usage.
- Its Common Name has the form `userName@hostName`. The host part
  identifies the cluster whose backups and WAL archive the client may
  access, and must match the `clusterName` of the
  `PluginConfiguration` using it.

The host part of the Common Name is load bearing. For Tier 1 base
backups a mismatch is rejected at connection time; for WAL streaming it
is not currently detected and can lead to WAL files being stored under
the wrong cluster path. See
[Cluster name override](plugin_configuration.md#cluster-name-override).

For a worked example creating the CA, the CA-backed issuer and a client
certificate together, see
[Create the certificates](quickstart.md#step-3-create-the-certificates).
