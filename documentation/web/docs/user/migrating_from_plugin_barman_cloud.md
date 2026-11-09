---
sidebar_position: 92
---

# Migrating from Plugin Barman Cloud

You can enable the Klio plugin on a cluster already protected by
[Plugin Barman Cloud](https://github.com/cloudnative-pg/plugin-barman-cloud/)
and run the two side by side.
To migrate without losing the ability to recover, keep both until Klio's
backups cover the recovery window you need, then remove Plugin Barman Cloud.

## Running both plugins

Plugin Barman Cloud keeps archiving WAL to its object store, while Klio
streams WAL to its server over its own replication slot:

```yaml
spec:
  postgresql:
    pg_hba:
      - local replication all peer map=local
  plugins:
    - name: barman-cloud.cloudnative-pg.io
      isWALArchiver: true
      parameters:
        barmanObjectName: <object-store>
    - name: klio.cnpg.io
      enabled: true
      parameters:
        pluginConfigurationRef: <plugin-configuration>
```

Each `Backup` and `ScheduledBackup` uses `method: plugin` and targets one
plugin through `.spec.pluginConfiguration.name`. CloudNativePG runs a
cluster's backups one at a time, whatever the plugin, so give the two plugins
different backup schedules.

While both plugins are enabled, either one can serve the WAL files that
`restore_command` requests, for example when a standby falls behind or a
former primary is rewound. CloudNativePG tries every enabled plugin that can
restore WAL, and uses the first that succeeds.

## Migration steps

1. Create the Klio `PluginConfiguration` and its client certificate, then
   add the Klio plugin and the `pg_hba` entry to the `Cluster` as shown
   above. See [Plugin Configuration](plugin_configuration.md).
2. Take a first Klio backup. Point-in-time recovery from Klio starts there.
3. Keep both plugins until Klio covers the required recovery window. During
   this phase, switch any replica clusters that follow this cluster through
   Plugin Barman Cloud to a Klio source.
4. Delete the Plugin Barman Cloud `ScheduledBackup` objects, then remove
   the plugin from `.spec.plugins`.

:::warning
Plugin Barman Cloud stops archiving as soon as the `Cluster` change in
step 4 is applied, before any instance restarts, so the WAL segment in
progress at that moment never reaches its archive.
Any WAL written after Plugin Barman Cloud is removed is only in Klio.
:::

:::info
Adding Klio in step 1 and removing Plugin Barman Cloud in step 4 each roll
out the instances.

When the primary is restarted in place or its Pod is recreated, if
applications keep their connections open, PostgreSQL waits up to
`smartShutdownTimeout` before shutting down. During a switchover, the
former primary shuts down in fast mode instead, which disconnects clients
immediately.
:::

## Recovery coverage

Each plugin can only recover to points covered by its own base backups and
WAL archive. During the migration, the two archives overlap:

| Recovery target                                   | Restore from        |
| ------------------------------------------------- | ------------------- |
| Before the first Klio backup completes            | Plugin Barman Cloud |
| From the end of the first Klio backup to step 4   | Either plugin       |
| From the WAL segment in progress at step 4 onward | Klio                |

Plugin Barman Cloud's coverage ends with the last WAL segment it archived
before step 4. Klio's coverage starts at the end of the oldest Klio backup
that its retention policy keeps, so it moves forward over time. Step 3 is
complete when that point is earlier than the oldest point you need to
recover to.

See [Point-in-Time Recovery](backup_and_restore.md#point-in-time-recovery-pitr)
for restoring from Klio.

## After the migration

Restoring to a point covered only by the Plugin Barman Cloud archive still
needs the plugin installed, with its `ObjectStore` and credentials in place.
Keep them until its retention window expires. The plugin no longer applies
its retention policy, so clean up the old backups in the object store
yourself.
