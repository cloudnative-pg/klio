---
sidebar_position: 8
---

# Backup and Restore

This guide explains how to take backups of PostgreSQL clusters managed by
CloudNativePG and restore them using Klio.

## Overview

Klio follows PostgreSQL's native physical backup and recovery mechanisms,
leveraging CloudNativePG's backup and restore capabilities through its
[`Backup` resource](https://cloudnative-pg.io/documentation/current/cloudnative-pg.v1/#postgresql-cnpg-io-v1-Backup)
and
[`ScheduledBackup` resource](https://cloudnative-pg.io/documentation/current/cloudnative-pg.v1/#postgresql-cnpg-io-v1-ScheduledBackup).

A working **online backup** is composed of:

- A **physical base backup**: A filesystem copy of the PostgreSQL data directory.
- A set of **WAL (Write-Ahead Log) files**: Continuous logs of all changes made
  to the database during the entire period of the base backup.

:::tip
It is recommended to periodically test backup restores to ensure correct
recovery procedures.
:::

## Prerequisites

Before performing backup and restore operations, you need a running
Klio server and a PostgreSQL cluster configured with the Klio plugin.
The [Quickstart](./quickstart.md) sets both up from scratch; see
[The Klio Server](./klio_server.md) and
[The Klio Plugin](./plugin_configuration.md) for the full
configuration reference.

## Taking a Backup

With the Klio plugin configured, you can take on-demand backups using
CloudNativePG's [`Backup` resource](https://cloudnative-pg.io/documentation/current/cloudnative-pg.v1/#postgresql-cnpg-io-v1-Backup)
or the [Kubectl plugin](https://cloudnative-pg.io/documentation/current/kubectl-plugin/#requesting-a-new-physical-backup)
for CNPG.

### Backup Target: Primary or Standby

Backups can be taken from the primary instance or from a standby, using
the `target` field of the `Backup` resource (`primary` or `standby`).
Running backups on a standby offloads I/O from the primary, at the cost
of the warning below.

:::warning
A standby cannot mark a backup as completed if the primary is idle. WAL
files are only shipped once the primary switches to a new WAL segment,
and an idle primary has no reason to trigger that switch. If the
primary receives no write traffic, a standby backup can stay stuck in
progress indefinitely. Ensure the primary has periodic write activity
before relying on standby backups on an otherwise idle cluster.
:::

### Create a Backup

You can trigger a new backup by creating a `Backup` resource.

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Backup
metadata:
  name: cluster-example-backup-20251027
  namespace: default
spec:
  method: plugin
  target: primary
  cluster:
    name: cluster-example
  pluginConfiguration:
    name: klio.cnpg.io
```

Apply the manifest:

```bash
kubectl apply -f backup.yaml
```

Alternatively, you can request a backup directly using the
 [`kubectl cnpg` plugin](https://cloudnative-pg.io/documentation/current/kubectl-plugin/#requesting-a-new-physical-backup):

```bash
kubectl cnpg backup cluster-example \
  --method plugin \
  --plugin-name klio.cnpg.io \
  --backup-target primary
```

If you don’t specify the `--backup-name` option, the `cnpg backup` command
automatically generates one using the format `<CLUSTER_NAME>-<YYYYMMDDhhmmss>`,
which is suitable in most cases.

For a complete list of available options, run:

```bash
kubectl cnpg backup --help
```

### Monitor Backup Progress

Check the backup status:

```bash
# Watch the backup status
kubectl get backup cluster-example-backup-20251027 -w

# Get detailed backup information
kubectl describe backup cluster-example-backup-20251027
```

A successful backup will show:

```
NAME                             AGE   CLUSTER           METHOD   PHASE       ERROR
cluster-example-backup-20251027  2m    cluster-example   plugin   Completed
```

### Scheduled Backups

You can schedule automatic backups using CloudNativePG's
[`ScheduledBackup` resource](https://cloudnative-pg.io/documentation/current/cloudnative-pg.v1/#postgresql-cnpg-io-v1-ScheduledBackup).

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: cluster-example-daily-backup
  namespace: default
spec:
  # Cron schedule: daily at 2:00 AM
  schedule: "0 0 2 * * *"
  method: plugin
  target: primary
  cluster:
    name: cluster-example
  pluginConfiguration:
    name: klio.cnpg.io
```

Apply the scheduled backup:

```bash
kubectl apply -f scheduled-backup.yaml
```

## Backup Retention and Maintenance

Klio automatically manages backup retention based on the
[retention policies](plugin_configuration.md#retention-policies) defined in the
`PluginConfiguration` referred by the `Cluster`. A backup that was
interrupted, or lost one of its snapshots, is not listed and cannot be
restored: see [Incomplete Backups](#incomplete-backups) below.

:::warning
Deleting a `Backup` resource through `kubectl` only removes the Kubernetes
object. The actual backup data in the Klio server will be retained according to
the retention policy.
:::

## Incomplete Backups

On each tier, a backup is a set of Kopia snapshots: one for `PGDATA`, one
for the control file, one per tablespace and, uploaded last, one for the
backup metadata. A backup that is not fully present on a tier is
*incomplete* there. `klio admin list-backups` and `klio backup list` only
show complete backups, and only complete backups can be restored.

A backup is incomplete on a tier when:

- it has no metadata snapshot. The metadata is uploaded last, so this is
  the shape of an upload still in progress, or of an upload that was
  interrupted;
- it has the metadata snapshot, but one of the parts the metadata expects
  is missing, for example because a snapshot was deleted by hand, or a
  relay to tier2 was interrupted.

### Status

Every backup attempt has its own name, so an interrupted attempt never
resumes under that name. Whether an incomplete backup can still complete
depends on the tier it is on:

- `PENDING`: on tier1, the latest attempt of its cluster, without
  metadata. It may still be uploading. Wait for it. If it is older than
  the backup interval of the cluster, the attempt died: delete it.
- `RECOVERABLE`: on tier2, complete on tier1. The relay copies to tier2
  whatever tier1 has, so the next backup of the cluster relayed to tier2
  copies the missing parts. A relay in progress looks like this too.
  Wait for the next relayed backup, or delete it: the relay copies it
  again.
- `UNRECOVERABLE`: it can never complete. It is an older tier1 attempt
  (a newer one started), a tier1 backup that lost a part, or a tier2
  backup with no complete copy on tier1. Delete it.

Klio never deletes an incomplete backup on its own, whatever the
retention policy: an unexpected one may be worth a look before it goes.
Its snapshots take space on the tier until deleted, though deduplication
keeps the cost low when a complete backup shares their content.

### Listing Incomplete Backups

```bash
kubectl exec -it klio-server-klio-0 -- \
  klio admin list-backups --incomplete
```

Run it on the server that owns tier1: the status of a tier2 backup
depends on what tier1 holds, so a server in
[read-only mode](concepts/architectures.md#read-only-server-mode)
refuses the command.

The output is a JSON array, empty when nothing is incomplete:

```json
[
  {
    "clusterName": "cluster-example",
    "backupName": "backup-20261027T140000Z",
    "tier": "tier2",
    "status": "RECOVERABLE",
    "startedAt": "2026-10-27T14:00:00.123456Z",
    "metadataPresent": true,
    "missingParts": ["pgdata"]
  }
]
```

`missingParts` lists the parts the metadata expects and are missing:
`pgdata`, `controldata` or `tablespace/<name>`. It is empty when the
metadata itself is missing, as nothing is known about the backup then.

The retention sweep, which runs every minute, also logs the incomplete
backups it finds, and the Klio server exports them in the
`klio.server.backup.incomplete_backups` gauge, broken down by
`cluster_name`, `tier` and `status` (see
[OpenTelemetry](opentelemetry.md)). A `PENDING` value is normal while a
backup runs: alert on it only once it has lasted longer than the backup
interval. An `UNRECOVERABLE` value never clears on its own.

### Deleting an Incomplete Backup

An incomplete backup is deleted like a complete one:

```bash
kubectl exec -it klio-server-klio-0 -- \
  klio admin delete-backup backup-20261027T140000Z \
    --cluster cluster-example --tier1 --tier2
```

When deleting a backup by hand from both tiers, delete the tier1 copy
first, or together with the tier2 one as above. The relay copies to tier2
whatever tier1 still has, so a tier2 copy deleted alone comes back at the
next relay of the cluster.

### Incomplete Backups and WAL Retention

A tier keeps the WAL files of every backup whose metadata it holds,
complete or not: dropping them cannot be undone, and a `RECOVERABLE`
backup needs them once the relay completes it. Delete an incomplete
backup to let WAL retention move past it. A backup without metadata
holds no WAL files, as its starting WAL is not known.

## Finding Your backupID for Recovery

To restore a specific backup, you need its backupID, otherwise Klio will
choose the latest one autonomously.
You can list all available, completed Backup resources using kubectl:

```bash
kubectl get backups -n <your-namespace>
```

Once you identify the backup you want to use, you can identify its backupID

```bash
kubectl get backup <backup_name> -n <your-namespace> -o jsonpath='{.status.backupId}'
```

## Restoring from a Backup

Klio supports restoring PostgreSQL clusters from backups using CloudNativePG's
recovery mechanism. Unlike traditional in-place recovery, Klio follows
CloudNativePG's approach of **bootstrapping a new cluster** from a backup,
which ensures data integrity and allows for flexible recovery scenarios.

### How Recovery Works

Klio integrates with CloudNativePG's recovery process by performing the
following actions during a restore:

1. **Restores the base backup**: Copies the physical backup data to the new
   cluster's data directory. Uses `klio restore` command under the hood.
1. **Restores WAL files**: Klio is configured to retrieve the WAL files
   required for PostgreSQL recovery as needed.
   Uses `klio get-wal` command under the hood.

The execution of these commands is driven by CloudNativePG's recovery
mechanism, which ensures that the PostgreSQL server starts correctly after
the restore.

A restored cluster operates independently of the original cluster. By default,
it will **not** perform backups unless you explicitly configure the Klio plugin
for backup operations in the new cluster's specification.

### Full Restore

To restore from a backup, create a new `Cluster` resource with a
`bootstrap.recovery` section that references the Klio plugin:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: cluster-restore
  namespace: default
spec:
  instances: 3

  # Bootstrap from a Klio backup
  bootstrap:
    recovery:
      source: source
      # OPTIONAL: Specify the backup to restore from
      # recoveryTarget:
      #   backupID: cluster-example-backup-YYYYMMDDHHMMSS

  # Reference the Klio plugin configuration
  externalClusters:
    - name: source
      plugin:
        name: klio.cnpg.io
        parameters:
          pluginConfigurationRef: klio-restore-config

  storage:
    size: 10Gi
```

:::note
Klio will choose the latest backup available in case the `backupID` field is
omitted.
:::

Create a corresponding `PluginConfiguration` that specifies which backup to
restore:

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: PluginConfiguration
metadata:
  name: klio-restore-config
  namespace: default
spec:
  # Connection details
  serverAddress: klio-server.default
  clientSecretName: cluster-example-klio-user
  serverSecretName: klio-server-tls

  # Required: the name of the original cluster that was backed up
  clusterName: cluster-example
```

The client credentials secret (`cluster-example-klio-user`) should contain the
necessary authentication information to access the Klio server. See the
[Klio plugin configuration guide](./plugin_configuration.md#client-credentials-secret)
for the requirements a client secret must satisfy.

:::note
The `clusterName` field in the `PluginConfiguration` and the `commonName`
of the certificate should match the name of the **original cluster** that
was backed up, not the name of the new restored cluster.
:::

Apply both resources:

```bash
kubectl apply -f restore-config.yaml
kubectl apply -f restored-cluster.yaml
```

### Point-in-Time Recovery (PITR)

Klio supports Point-in-Time Recovery, allowing you to restore your database
to a specific moment in time rather than the latest available state. This is
useful for recovering from accidental data deletion or corruption.

The process involves specifying a recovery target in the `Cluster` resource.
The available recovery targets are described in the
[CloudNativePG documentation](https://cloudnative-pg.io/documentation/current/recovery/#recovery-targets).

#### Example: recover to a `targetTime`

Restore to a specific timestamp:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: cluster-pitr
spec:
  bootstrap:
    recovery:
      source: source
      # Recover to a specific point in time
      recoveryTarget:
        targetTime: "2025-11-06 15:00:00.0000+00"
  # other cluster spec fields...
```

:::info
The target of a point in time recovery must fall between the time the base
backup was completed and the time of the latest transaction recorded in the
available WAL files.
:::

:::note
During the Point in Time Recovery, if `targetTime` or `targetLSN` are specified,
Klio will automatically choose the closest backup for the PITR, if not defined
with the `backupID` field.
:::
