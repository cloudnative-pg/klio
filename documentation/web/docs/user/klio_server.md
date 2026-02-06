---
sidebar_position: 5
---

# The Klio Server

The Klio server is a central component of the Klio backup solution. It is
defined as the `Server` custom resource in Kubernetes, which creates a
StatefulSet running the Klio server application.

The Klio server is composed of two main containers:

- `base`: Manages full and incremental backups using Kopia.
- `wal`: Receives the stream of PostgreSQL Write-Ahead Logs (WAL).

An additional init container, `init`, is responsible for initializing the
Kopia repository and setting up the necessary configuration.

The base backups and WAL files are stored in multiple PersistentVolume attached
to the Klio server pod in the `/data/base` and `/data/wal` directories, respectively.

An additional cache defined by a PersistentVolume is used for the Kopia cache.
This cache allows Kopia to quickly browse repository contents without
having to download from the storage location.

## Storage Tiers

### Tier 1: Local Storage

Tier 1 uses local `PersistentVolumes` for immediate data access.
This is the primary landing zone for backups and WAL files,
providing the fastest recovery times.

### Tier 2: Remote Object Storage

:::warning Work in Progress
Tier 2 functionality is currently under heavy development and should be
considered experimental. The features described below are subject to change.
:::

Tier 2 offloads data to S3-compatible object storage.
This is used for long-term retention and disaster recovery.
When Tier 2 is enabled, the server uses a work queue to manage
the asynchronous transfer of data from the local environment to the cloud.

### The Work Queue

If both Tier 1 and Tier 2 are configured, it is mandatory to configure
a work queue in the klio Server resource.
The work queue is backed by NATS JetStream with file storage on a separate
`PersistentVolume mounted` at `/queue`.
When a WAL file is received, the server publishes a notification to the queue,
enabling asynchronous processing. This ensures that the primary backup flow
is not slowed down by network latency to remote object storage.

## Storage Requirements

The Klio Server uses three distinct PersistentVolumeClaims (PVCs), each
serving a different purpose. Understanding what each PVC contains helps you
size them appropriately for your environment.

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

The queue PVC is only required when both Tier 1 and Tier 2 are configured.
It stores the NATS JetStream work queue used for asynchronous Tier 2
replication.

## Setting up a new Klio server

Setting up a Klio server involves creating a `Server` resource along with the
required Kubernetes secrets and certificates.

### Prerequisites

Before setting up a Klio server, ensure you have:

- A Kubernetes cluster with the Klio operator installed
- `kubectl` configured to access your cluster
- [cert-manager](https://cert-manager.io/) installed for certificate
  management (recommended)
- Enough storage resources for the data and cache PersistentVolumeClaims
- Enough storage resources for the queue PersistentVolumeClaim

### Required Components

A Klio server setup requires the following components:

1. **Server Resource**: The main `Server` custom resource
1. **TLS Certificate**: For secure communication
1. **Encryption Password**: For encrypting backup data at rest
1. **CA Certificate**: For client authentication via mTLS
1. **Storage**: PersistentVolumeClaims for data, cache, and queue

### Step-by-step setup

#### 1. Create the Encryption Key Secret

The encryption key is used to encrypt backup data at rest:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-server-encryption
  namespace: default
type: Opaque
data:
  encryptionKey: "bXktc2VjdXJlLWtleQ==" # my-secure-key
```

Apply the secret:

```bash
kubectl apply -f encryption-secret.yaml
```

:::tip
Use a strong, randomly generated key. This key is critical for
data security and recovery.
:::

#### 2. Create CA Certificate

Using cert-manager, a CA certificate can be created by using the following
Certificate resource:

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

Apply the CA configuration with:

```
kubectl apply -f ca-configuration.yaml
```

In the previous example, the CA to be used for authentication is signed by a
self-signed issuer. This doesn't pose any security issue as this CA is only
used internally and trust is established through configuration.

The primary concern is the relationship between the client and the certificates
signed by the CA.

:::info
The usage of a self-signed CA is not required by the Klio server. If your
PKI infrastructure already includes a CA for this scope, that CA can be used
for the Klio server, too.
:::

#### 3. Create TLS Certificate

Using cert-manager, create a self-signed certificate (for development) or use
your organization's certificate issuer:

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

Apply the certificate configuration:

```bash
kubectl apply -f tls-certificate.yaml
```

:::info
For production environments, use certificates signed by your organization's
Certificate Authority (CA) or a trusted public CA instead of self-signed
certificates.
:::

#### 4. Create the Server Resource

Now create the main `Server` resource:

<!-- x-release-please-start-version -->
```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: Server
metadata:
  name: my-server
  namespace: default
spec:
  # Container image for the Klio server
  image: ghcr.io/enterprisedb/klio:v0.0.11
  imagePullPolicy: IfNotPresent
  imagePullSecrets: []  # Add image pull secrets if needed

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
    # Encryption key reference
    encryptionKey:
      name: my-server-encryption
      key: encryptionKey

  # Queue storage configuration (for NATS work queue)
  # It can be added only if both tier1 and tier2 are configured
  queue:
    pvcTemplate:
      storageClassName: standard  # Adjust to your storage class
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 10Gi  # Adjust based on queue volume needs

  # tier 2 configuration
  tier2:
    # Cache storage configuration
    cache:
      pvcTemplate:
        resources:
          requests:
            storage: 1Gi
        accessModes:
          - ReadWriteOnce
    # Encryption key reference. Can differ from tier1 encryption key.
    encryptionKey:
      name: my-server-encryption
      key: encryptionKey
    # S3 access configuration
    s3:
      prefix: klio
      bucketName: klio-bucket
      endpoint: https://rustfs:9000
      region: us-east-1
      accessKeyId:
        name: rustfs
        key: RUSTFS_ACCESS_KEY
      secretAccessKey:
        name: rustfs
        key: RUSTFS_SECRET_KEY
      customCaBundle:
        name: rustfs-tls
        key: tls.crt
```
<!-- x-release-please-end -->

Apply the Server resource:

```bash
kubectl apply -f klio-server.yaml
```

#### 5. Verify the Server is Running

Check the status of your Klio server:

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
merged with the default Klio server containers `base` and `wal`. If you do not
need to add containers or modify the default ones, you must still include an
empty list.
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

See [Reserving Nodes for Klio Workloads](architectures.md#reserving-nodes-for-klio-workloads)
for details on node tainting.

### Monitoring

Refer to the [OpenTelemetry](./opentelemetry.md#klio-server-with-opentelemetry)
documentation for setting up monitoring and telemetry for the Klio server.

## Encryption

Klio implements encryption at rest for both base backups and WAL files to
ensure data security throughout the backup lifecycle.

### Base Backups Encryption

Base backups are encrypted by Kopia using the encryption password provided in
the `encryptionKey` secret references. Kopia handles encryption transparently.

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

### Encryption Password Rotation

Currently, encryption key rotation is not supported. To change the
encryption key, you would need to:

1. Create a new Klio server with a new encryption key
1. Perform new base backups to the new server
1. Migrate to using the new server

:::tip
Choose a strong encryption key from the start. Use a password manager or
key management system to generate and store a cryptographically secure key
(recommended: 32+ random characters).
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

## Authentication

Klio uses mTLS Authentication for securing access to both the base backup server
and the WAL streaming server. Authentication is handled by verifying the client
certificates against the CA certificate which has been created when configuring
the Klio server.

### Creating a client-side certificate

To create a client-side certificate, you need a issuer that will sign all the
certificates with a CA known by the Klio server. Supposing that such a issuer is
called `server-sample-ca` and available in the current namespace, you can create
a client certificate with the following Certificate object:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: client-sample-tls
spec:
  secretName: client-sample-tls
  commonName: klio@cluster-1

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

If used the example proposed in the [server configuration documentation
page](#2-create-ca-certificate), the issuer can be created with:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: server-sample-ca
spec:
  ca:
    secretName: server-sample-ca
```
