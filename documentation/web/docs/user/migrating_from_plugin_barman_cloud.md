---
sidebar_position: 8.5
---

# Migrating from Plugin Barman Cloud

You can enable the Klio plugin on a cluster already protected by
[Plugin Barman Cloud](https://github.com/cloudnative-pg/plugin-barman-cloud/) and run the two side by side.
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

Each `Backup` and `ScheduledBackup` targets one plugin through
`.spec.pluginConfiguration.name`. CloudNativePG runs a cluster's backups one
at a time, whatever the plugin, so give the two plugins different backup
schedules.

## Migration steps

1. Create the Klio `PluginConfiguration` and its client certificate, then
   add the Klio plugin and the `pg_hba` entry to the `Cluster` as shown
   above. See [Plugin Configuration](plugin_configuration.md).
2. Take a first Klio backup. Point-in-time recovery from Klio starts there.
3. Keep both plugins until Klio covers the required recovery window. During
   this phase, switch any replica clusters that follow this cluster through
   Plugin Barman Cloud to a Klio source.
4. Close the Plugin Barman Cloud archive: run `SELECT pg_switch_wal()` on
   the primary and wait until `pg_stat_archiver` reports that segment as
   archived.
5. Remove Plugin Barman Cloud from `.spec.plugins` and delete its
   `ScheduledBackup` objects.

:::warning
Setting `isWALArchiver: false` does not stop Plugin Barman Cloud from
archiving; only removing it from `.spec.plugins` does. If you skip step 4,
the transactions in the last WAL segment are missing from its archive.
:::

:::info
Adding Klio and removing Plugin Barman Cloud each roll out the instances
and restart the primary in place. If applications keep their connections
open, the restart waits up to `smartShutdownTimeout` before PostgreSQL shuts
down.
:::

## After the migration

Restoring to a point covered only by the Plugin Barman Cloud archive still
needs the plugin installed, with its `ObjectStore` and credentials in place.
Keep them until its retention window expires. The plugin no longer applies
its retention policy, so clean up the old backups in the object store
yourself.
