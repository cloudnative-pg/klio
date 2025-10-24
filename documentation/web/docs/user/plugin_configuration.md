---
sidebar_position: 6
---

# The Klio Plugin

The Klio plugin for CloudNativePG allows you to leverage the backup and WAL
streaming capabilities of Klio for your PostgreSQL clusters managed by
CloudNativePG. It will add two containers to each PostgreSQL instance pod:

- A `klio-plugin` container that handles backup creation and management
- A `klio-wal` container that streams WAL files to the Klio server in real-time

## Configuration

The Klio plugin integrates with CloudNativePG through the CNPG-I (CloudNativePG
Interface) specification, enabling Klio to manage backups and WAL streaming for
your PostgreSQL clusters. To use Klio with a CloudNativePG cluster, you need to:

1. Create a `PluginConfiguration` resource that defines how to connect to the
   Klio server
1. Reference the plugin in your `Cluster` resource specification

## Prerequisites

Before configuring a cluster to use the Klio plugin, ensure you have:

- A running Klio `Server` resource deployed in your namespace
- Client credentials (username and password) stored in a Kubernetes Secret
- The server's TLS certificate available in a Secret

## Creating a PluginConfiguration resource

The `PluginConfiguration` custom resource defines how the Klio plugin connects
to and communicates with the Klio server. This resource contains connection
details, authentication credentials, and optional configuration for metrics,
profiling, and backup retention policies.

### Basic example

Here's a minimal `PluginConfiguration` example:

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: klio-plugin-config
  namespace: default
spec:
  serverAddress: klio-server.default
  clientSecretName: klio-client-credentials
  serverSecretName: klio-server-tls
```

### Client credentials secret

The client credentials must be stored in a Kubernetes Secret of type
`kubernetes.io/basic-auth` or a generic Secret containing `username` and
`password` keys:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: klio-client-credentials
  namespace: default
type: kubernetes.io/basic-auth
data:
  username: a2xpbw==       # base64-encoded username
  password: cGFzc3dvcmQ=   # base64-encoded password
```

These credentials will be used by the Klio plugin to authenticate with the
Klio server, and must match a user configured in the Klio server htpasswd file.

### Server Address

The `serverAddress` field specifies where the Klio server can be reached. This
can be:

- A Kubernetes service name: `klio-server.default` (within the same namespace)
- A fully qualified domain name: `klio-server.default.svc.cluster.local`
- An external address: `klio.example.com`

Connections will be done using the default ports of the Klio base and WAL
servers, respectively 51515 and 52000.

### TLS configuration

The `serverSecretName` field references a Secret containing the TLS certificate
used to secure communication with the Klio server. This is the same
certificate configured on the `Server` resource.

## Configuring a Cluster to use the Klio plugin

Once you have created a `PluginConfiguration`, reference it in your CloudNativePG
`Cluster` resource:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: my-postgres-cluster
  namespace: default
spec:
  instances: 3

  postgresql:
    pg_hba:
      - local replication all peer # Allow replication connections locally

  plugins:
    - name: klio.cnpg.io
      enabled: true # Activate the Klio plugin (default)
      parameters:
        pluginConfigurationRef: klio-plugin-config

  storage:
    size: 10Gi
```

To be able to stream WAL files, ensure that your PostgreSQL configuration
allows local replication connections. You can do this by adding an entry to the
`pg_hba` section, as shown in the example above.

### Plugin parameters

The `plugins` section in the `Cluster` specification requires:

- **name**: Must be set to `klio.cnpg.io` to identify the Klio plugin
- **enabled**: Set to `true` to activate the plugin. This is the default value.
- **parameters.pluginConfigurationRef**: The name of your `PluginConfiguration` resource

:::note
Even though the Klio plugin is used to archive WAL files on the Klio server,
it does not use the `archiveCommand` parameter in the PostgreSQL configuration,
as the WAL are streamed directly to the Klio server. Thus, you must not set
`isWALArchiver: true` in the plugin configuration.
:::

## Advanced configuration options

The `PluginConfiguration` resource supports several advanced options to
customize the plugin's behavior.

### Retention policies

Define how long backups should be retained by configuring the retention policy:

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: klio-plugin-config
spec:
  serverAddress: klio-server.default
  clientSecretName: klio-client-credentials
  serverSecretName: klio-server-tls
  retention:
    keepLatest: 5
    keepHourly: 12
    keepDaily: 7
    keepWeekly: 4
    keepMonthly: 6
    keepAnnual: 2
```

Except for `keepLatest`, each option defines how many backups to retain
for the specified time period. For example, `keepDaily: 7` means that we should
retain at most one backup for each of the past 7 days.

If multiple backups exist within the same time bucket, the most recent one is
kept, unless preserved by a different *keep* rule. Backups that are not
retained by any rule are deleted. Rule evaluation is done when a new backup is
taken.

The Klio server will automatically delete WAL files that are no longer needed
for recovery by any retained backup.

All retention settings are optional. For each unspecified retention level,
the default Kopia value is applied:

```yaml
keepLatest: 10
keepHourly: 48
keepDaily: 7
keepWeekly: 4
keepMonthly: 24
keepAnnual: 1
```

Set a rule to `0` to disable that retention level.

### Cluster name override

By default, the plugin uses the name of the CloudNativePG `Cluster` resource.
You can override this if needed:

```yaml
spec:
  clusterName: my-custom-cluster-name
```

This can be useful working with backups from different clusters, for example
when restoring clusters or configuring replica clusters.

### Restore configuration

When performing a restore, you can specify which backup to use:

```yaml
spec:
  backupRef: backup-resource-name
```

Alternatively, you can specify a backup by its internal ID:

```yaml
spec:
  backupId: backup-YYYYMMDDHHMMSS
```

:::note
The `backupRef` and `backupId` fields are mutually exclusive. You can only
specify one of them.
:::

### Observability

See the [OpenTelemetry observability](./opentelemetry.md) section for more
details on how to monitor the Klio plugin using OpenTelemetry.

#### Metrics Endpoints

Klio exposes Prometheus-compatible metrics for monitoring. You can configure
separate metric endpoints for different components:

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: klio-plugin-config
spec:
  serverAddress: klio-server.default
  clientSecretName: klio-client-credentials
  serverSecretName: klio-server-tls
  metricsAddressInstance: ":8085"
  metricsAddressSendWal: ":8090"
  metricsAddressRestore: ":8091"
```

The available metrics endpoints are:

- `metricsAddressInstance`: Metrics from the `klio-plugin` container
- `metricsAddressSendWal`: Metrics from the `klio-wal` container
- `metricsAddressRestore`: Metrics from the restore container

Each address should specify a port in the format `:PORT`. These endpoints can
be scraped by Prometheus or other monitoring systems.

:::warning
These metrics endpoints could be removed in future Klio releases, in favor
of OpenTelemetry-based metrics.
:::

### Performance profiling

Enable the pprof HTTP endpoint for performance profiling and troubleshooting:

```yaml
spec:
  pprof: true
```

When enabled, the pprof endpoint is exposed and can be used with Go's profiling
tools to analyze CPU usage, memory allocation, goroutines, and other runtime
metrics.

:::warning
Only enable pprof in development or testing environments, or when actively
troubleshooting performance issues. It should not be enabled in production
unless necessary.
:::
