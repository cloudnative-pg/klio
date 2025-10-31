---
sidebar_position: 7
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

:::note
The PostgreSQL's point in time recovery (PITR) feature is achievable by
continuously collecting all the WAL files generated since the base backup
up to the moment the recovery is requested.

:::important
It is recommended to periodically test backup restores to ensure correct
recovery procedures.
:::

:::warning
The Klio MVP does not currently verify the presence of all required WAL files
for a given backup. This limitation will be resolved before the GA release.
:::

## Prerequisites

Before performing backup and restore operations, ensure you have:

- A running [Klio server](./klio_server.md) with proper configuration
- A PostgreSQL cluster configured with the [Klio plugin](./plugin_configuration.md)

## Taking a Backup

With the Klio plugin configured, you can take on-demand backups using
CloudNativePG's [`Backup` resource](https://cloudnative-pg.io/documentation/current/cloudnative-pg.v1/#postgresql-cnpg-io-v1-Backup)
or the [Kubectl plugin](https://cloudnative-pg.io/documentation/current/kubectl-plugin/#requesting-a-new-physical-backup)
for CNPG.

### Create a Backup

You can trigger a new backup by creating a `Backup` resource.

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

Apply the manifest:

```bash
kubectl apply -f backup.yaml
```

Alternatively, you can request a backup directly using the
 [`kubectl cnpg` plugin](https://cloudnative-pg.io/documentation/current/kubectl-plugin/#requesting-a-new-physical-backup):

```bash
kubectl cnpg backup my-cluster \
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
kubectl get backup my-cluster-backup-20251027 -w

# Get detailed backup information
kubectl describe backup my-cluster-backup-20251027
```

A successful backup will show:

```
NAME                          AGE   CLUSTER      METHOD   PHASE       ERROR
my-cluster-backup-20251027    2m    my-cluster   plugin   Completed
```

### Scheduled Backups

You can schedule automatic backups using CloudNativePG's
[`ScheduledBackup` resource](https://cloudnative-pg.io/documentation/current/cloudnative-pg.v1/#postgresql-cnpg-io-v1-ScheduledBackup).

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: my-cluster-daily-backup
  namespace: default
spec:
  # Cron schedule: daily at 2:00 AM
  schedule: "0 0 2 * * *"
  method: plugin
  target: primary
  cluster:
    name: my-cluster
  pluginConfiguration:
    name: klio.cnpg.io
```

Apply the scheduled backup:

```bash
kubectl apply -f scheduled-backup.yaml
```

## Backup Retention and Maintenance

Klio automatically manages backup retention based on the
[retention policy](plugin_configuration.md#retention-policies) defined in the
`PluginConfiguration` referred by the `Cluster`.

:::important
Deleting a `Backup` resource through `kubectl` only removes the Kubernetes
object. The actual backup data in the Klio server may be retained according to
the retention policy.
:::

## Restoring from a Backup

TODO
