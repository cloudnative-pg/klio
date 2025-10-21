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

An additional cache defined by a PersistentVolume is used for the Kopia cache. This cache allows Kopia to
quickly browse repository contents without having to download from the storage
location.

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

### Required Components

A Klio server setup requires the following components:

1. **Server Resource**: The main `Server` custom resource
2. **TLS Certificate**: For secure communication
3. **Encryption Password**: For encrypting backup data at rest
4. **User Credentials**: For authentication via htpasswd
5. **Admin User Credentials**: Optional admin user for Kopia operations
6. **Storage**: PersistentVolumeClaims for data and cache

### Step-by-step setup

#### 1. Create the Encryption Password Secret

The encryption password is used to encrypt backup data at rest:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-server-encryption
  namespace: default
type: Opaque
data:
  password: "bXktc2VjdXJlLXBhc3N3b3Jk" # my-secure-password
```

Apply the secret:

```bash
kubectl apply -f encryption-secret.yaml
```

:::tip
Use a strong, randomly generated password. This password is critical for
data security and recovery.
:::

#### 2. Create user credentials

Create user credentials for authentication using the `htpasswd` format.
The secret must contain an `htpasswd` key with base64-encoded credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-server-users
  namespace: default
type: Opaque
data:
  # Format: username:hashed-password
  # Example: klio@my-cluster with password
  htpasswd: a2xpb0BteS1jbHVzdGVyOiQyeSQwNSRVaXpuRzhnRzhBejZIS1FnL01OS2FPb0NyQUplM2RqNmg5Y3ZhT1drL0VJZHo5OXJhczNnYQoK
```

To generate `htpasswd` entries:

```bash
# Generate password hash and base64 encode the output for the secret
htpasswd -nbB klio@my-cluster "my-password" | base64 -w0
```

Apply the secret:

```bash
kubectl apply -f user-credentials.yaml
```

:::note
You can add multiple users by including additional lines in the `htpasswd` key,
each formatted as `username:hashed-password`.
:::

:::warning
Changing the secret will require restarting the Klio server to pick up the new
credentials.
:::

#### 3. (Optional) Create Admin User Credentials

If you need admin access to the underlying Kopia server web interface
(mostly for debugging purposes), define the secret as follows:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-server-adm
  namespace: default
type: kubernetes.io/basic-auth
data:
  username: "YWRtaW4=" # admin
  password: "YWRtaW4tcGFzc3dvcmQ=" # admin-password
```

Apply the secret:

```bash
kubectl apply -f admin-credentials.yaml
```

#### 4. Create TLS Certificate

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
For production environments, use certificates signed by your organization's Certificate Authority (CA) or a trusted public CA instead of self-signed certificates.
:::

#### 5. Create the Server Resource

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
  image: ghcr.io/enterprisedb/klio:v0.0.5
  imagePullPolicy: IfNotPresent
  imagePullSecrets: []  # Add image pull secrets if needed

  # TLS configuration
  tlsSecretName: my-server-tls

  # Encryption password reference
  password:
    name: my-server-encryption
    key: password

  # User credentials reference
  users:
    name: my-server-users

  # Optional: Admin user for Kopia operations
  baseConfiguration:
    adminUser:
      name: my-server-adm

  # Cache storage configuration
  cacheConfiguration:
    pvcTemplate:
      storageClassName: standard  # Adjust to your storage class (use 'kubectl get storageclass' to see available options)
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 10Gi  # Adjust based on your needs

  # Data storage pvcTemplate (for backups and WAL)
  dataConfiguration:
    pvcTemplate:
      storageClassName: standard  # Adjust to your storage class (use 'kubectl get storageclass' to see available options)
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 100Gi  # Adjust based on your backup needs

  # Optional: Resource requirements
  resources:
    requests:
      memory: "1Gi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "2000m"
```
<!-- x-release-please-end -->

Apply the Server resource:

```bash
kubectl apply -f klio-server.yaml
```

#### 6. Verify the Server is Running

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
the `password` secret reference. Kopia handles encryption transparently.

The encryption password is set during repository initialization and is required
for all subsequent backup and restore operations.

:::warning Critical
Store the encryption password securely. Loss of this password means permanent
loss of access to all backup data. There is no password recovery mechanism.
:::

### WAL Files Encryption

WAL files are encrypted using a master key derivation system with authenticated
encryption. The encryption process works as follows:

1. **Master Key Generation**: A 32-byte master key is derived from the encryption
   password using PBKDF2
2. **Key Enveloping**: The master key itself is encrypted using AES-256-GCM with a
   password-derived encryption key to protect the key at rest
3. **Per-File Encryption**: Each WAL file is compressed and then encrypted using
   the master key with authenticated encryption before being stored

WAL files are first compressed using Snappy S2 compression, then encrypted to ensure both space
efficiency and security.

The same encryption password used for base backups encrypts the WAL files,
ensuring a unified security model across all backup artifacts.

### Encryption Password Rotation

Currently, encryption password rotation is not supported. To change the
encryption password, you would need to:

1. Create a new Klio server with a new encryption password
2. Perform new base backups to the new server
3. Migrate to using the new server

:::tip
Choose a strong encryption password from the start. Use a password manager or
key management system to generate and store a cryptographically secure password
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

Klio uses HTTP Basic Authentication for securing access to both the base backup
server and the WAL streaming server. Authentication is handled through htpasswd
files, a widely-used format for storing username and password credentials.

### How Authentication Works

Both the `base` and `wal` containers require authentication:

- **Base Container**: Authenticates Kopia client connections for backup and
  restore operations
- **WAL Container**: Authenticates PostgreSQL instances streaming WAL files via
  gRPC

Credentials are validated against the htpasswd file mounted from the Kubernetes
secret specified in `.spec.users`.

### User Credentials Format

The authentication secret must contain an `htpasswd` key with base64-encoded
credentials in the standard htpasswd format:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-server-users
  namespace: default
type: Opaque
data:
  htpasswd: <base64-encoded-htpasswd-content>
```

### Managing Multiple Users

You can define multiple users in the same htpasswd file. Each line represents
one user in the format `username:hashed-password`:

```bash
# Create htpasswd file with multiple users
htpasswd -nbB klio@cluster-1 "password1" > htpasswd-file
htpasswd -nbB klio@cluster-2 "password2" >> htpasswd-file
htpasswd -nbB restore-user@cluster-1 "password3" >> htpasswd-file

# Base64 encode and use in secret
cat htpasswd-file | base64 -w 0
```

Each PostgreSQL cluster or client can use different credentials, allowing for:

- **Cluster isolation**: Each cluster uses its own credentials
- **Access auditing**: Different users for different purposes (backup, restore, etc.)
- **Credential rotation**: Individual credentials can be updated independently

### Admin User (Optional)

The optional admin user (`.spec.baseConfiguration.adminUser`) provides access to
the Kopia web interface for administrative and debugging purposes. This is
separate from the regular user authentication:

```yaml
spec:
  baseConfiguration:
    adminUser:
      name: my-server-adm  # Reference to kubernetes.io/basic-auth secret
```

The admin user secret must be of type `kubernetes.io/basic-auth`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-server-adm
type: kubernetes.io/basic-auth
data:
  username: <base64-encoded-username>
  password: <base64-encoded-password>
```

:::info
The admin user is primarily intended for debugging and should be used sparingly
in production environments. Regular backup and restore operations use the
htpasswd-based credentials.
:::

### Credential Updates

To update user credentials:

1. Update the htpasswd secret with new credentials
2. Restart the Klio server pod to pick up the changes:

```bash
kubectl rollout restart statefulset my-server-klio -n default
```

:::tip Security Best Practices
- Use strong, unique passwords for each user
- Rotate credentials periodically
- Use the bcrypt algorithm (`-B` flag) when generating htpasswd entries
- Store secrets securely using your organization's secrets management solution
- Consider using different credentials for each PostgreSQL cluster
:::
