---
sidebar_position: 91
---

# Upgrade Notes

This page lists version-specific changes that may require
manual action when upgrading Klio. For the upgrade procedure,
see the [Helm chart page](helm_chart.mdx#upgrades).

## 0.0.20 to 0.0.21

### Upgrade procedure

This release changes both the `Server` and the `PluginConfiguration` CRDs
in incompatible ways, and every `Server` must be migrated to a new storage
layout by hand. Klio cannot archive WAL files or take backups until each
`Server` is migrated, so run the whole procedure in a maintenance window,
in this order:

1. **Prepare.** Remove any `klio-restore` entry from your
   `PluginConfiguration` resources (see
   [The `klio-restore` container is gone](#the-klio-restore-container-is-gone)),
   work out the PVC name, size and StorageClass of each `Server` (see
   [Migrating from the Multi-PVC Model](#migrating-from-the-multi-pvc-model)),
   and write the updated `Server` and `PluginConfiguration` manifests.

1. **[Upgrade the CRDs](helm_chart.mdx#upgrade-the-crds).** From this
   moment every `Server` on the old layout is down, even before the
   operator is upgraded. PostgreSQL keeps the WAL files in the `klio`
   replication slot until the `Server` is back, so make sure the
   PostgreSQL volumes have room for them.

1. **[Upgrade the operator](helm_chart.mdx#upgrade-the-operator).** The
   PostgreSQL instance pods of every cluster using Klio are restarted to
   roll out the new `klio-plugin` sidecar.

1. **Migrate each `Server`** to the unified PVC and apply its updated
   manifest, following
   [Migrating from the Multi-PVC Model](#migrating-from-the-multi-pvc-model).

1. **Update each `PluginConfiguration`** to the new retention format (see
   [Klio-managed retention policies](#klio-managed-retention-policies)).

1. **Verify.** Take a backup of each cluster and check that it completes.

The old manifests are not accepted any more: applying a `Server` or
`PluginConfiguration` manifest written for 0.0.20 fails with errors such as
`unknown field "spec.queue"` or
`unknown field "spec.tier1.retention.keepLatest"`.

### Klio-managed retention policies

Retention is now enforced by the Klio server, which periodically evaluates
each cluster's backups against its policy and deletes the ones that fall
outside it, instead of delegating the decision to Kopia.

- The `retention` block of the `PluginConfiguration` changed shape. The
  Kopia-style `keepLatest`, `keepHourly`, `keepDaily`, `keepWeekly`,
  `keepMonthly` and `keepAnnual` fields are replaced by a single `latest`
  field, which keeps the given number of most recent backups. Update any
  `tier1.retention` and `tier2.retention` blocks accordingly. Omit the block,
  or set `latest` to `0`, to keep every backup.
- Once the new CRDs are applied, Kubernetes silently drops the old `keep*`
  fields from existing `PluginConfiguration` resources, so Klio keeps every
  backup until you set `latest`.
- Kopia's own retention is disabled: on start, the Klio server sets the
  global Kopia retention policy of each tier to keep every snapshot and
  resets the per-source retention policies written by earlier versions to
  inherit it. No manual action is required.
- `klio retention set` no longer accepts the `--keep-*` flags. The policy
  is read from the configuration and sent to the server.

### The `klio-restore` container is gone

The `klio-plugin` sidecar now also serves the restore while a cluster
bootstraps from a Klio backup, so Klio no longer injects a separate
`klio-restore` container into the recovery Job. The `PluginConfiguration`
CRD only accepts `klio-plugin` in `spec.containers`: a `PluginConfiguration`
that still lists a `klio-restore` entry is rejected on its next change to
`spec.containers`, and
its status cannot be written on Kubernetes versions without CRD validation
ratcheting. Remove the `klio-restore` entry and move any customization you
need onto `klio-plugin` before upgrading.

### Migrating from the Multi-PVC Model

Klio servers created until v0.0.20 used four separate
PersistentVolumeClaims per server: `data`, `cachetier1`, `cachetier2`,
and `queue`. The current `Server` CRD no longer accepts that shape —
`spec.storage.pvcTemplate` is mandatory, and the old `tier1.data`,
`tier1.cache`, `tier2.cache`, and top-level `queue` fields no longer
validate. There is no automatic conversion: migrating an existing
Server to the unified PVC is a manual procedure.

Only `data` (the actual backups and WAL) and `queue`
(pending, not-yet-processed tasks) contain information that should be
moved to the new PVC. `cachetier1` and `cachetier2` will be automatically
recreated and populated on demand.

:::warning Plan for immediate, cluster-wide downtime on upgrade
Applying the 0.0.21 CRDs deletes the StatefulSet for
**every** Server still on the old PVC layout, the
moment the operator reconciles them — not when you get
around to migrating a given server. There is no way to defer or stage
this per server. Work out the PVC name and StorageClass for each
server (step 2 below) and schedule a maintenance window *before*
applying the CRDs, not after.

Between the CRD upgrade and the Server rewrite (step 4), the
`Server` object shows no status and emits no event: the only signal
that a given server is affected is an operator log line such as
`failed to reconcile statefulset ...
spec.volumeClaimTemplates[0].spec.accessModes: Required value`.
:::

1. **Applying the 0.0.21 CRDs will delete the StatefulSets and
   the pods associated with the `Server` resources**.
   The old PVCs survive this because the StatefulSet's
   `persistentVolumeClaimRetentionPolicy` is `Retain`.

1. **Create a PVC named to match what the StatefulSet will
   adopt.** Following the StatefulSet PVC naming convention
   `<volumeClaimTemplateName>-<statefulSetName>-<ordinal>`, where the
   unified volume claim template is named `klio` and the StatefulSet is
   named `<server-name>-klio`, the PVC to create is
   `klio-<server-name>-klio-0` (for example, `klio-klio-server-klio-0`
   for a server named `klio-server`). Give it
   a size at least as large as the old total across `data`, both
   caches, and `queue` combined.

   The operator hardcodes this name: the StatefulSet always looks for
   `klio-<server-name>-klio-0`.

   :::warning Getting this name wrong orphans the copied data
   The StatefulSet provisions a brand new, empty PVC instead of
   adopting the one you pre-created, and the copied data is silently
   orphaned on a PVC nothing mounts. After migrating, confirm the PVC
   is the one adopted by running:

   ```shell
   kubectl get pvc -l klio.cnpg.io/klio-server=<server-name>
   ```
   :::

   For a server named `<server-name>`, the PVC manifest looks like:

   ```yaml
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: klio-<server-name>-klio-0
     labels:
       klio.cnpg.io/klio-server: <server-name>
   spec:
     accessModes:
       - ReadWriteOnce
     storageClassName: <same StorageClass as the old PVCs>
     resources:
       requests:
         storage: <old data + cachetier1 + cachetier2 + queue sizes, combined>
   ```

1. **Run a one-off Job to copy the data.** Mount the old `data` PVC,
   and the old `queue` PVC if present, read-only, alongside the new PVC
   (read-write), and copy:
   - the old `data` PVC's contents into the new PVC's `data/`
   - the old `queue` PVC's contents into the new PVC's `queue/`

   For a server named `<server-name>`, following the old PVC naming
   convention `<volumeClaimTemplateName>-<statefulSetName>-<ordinal>`
   (`data-<server-name>-klio-0`, `queue-<server-name>-klio-0`), and the new
   PVC pre-created in the previous step (`klio-<server-name>-klio-0`):

   ```yaml
   apiVersion: batch/v1
   kind: Job
   metadata:
     name: <server-name>-pvc-migration
   spec:
     template:
       spec:
         restartPolicy: Never
         containers:
           - name: migrate
             image: busybox
             command:
               - sh
               - -c
               - |
                 set -e
                 mkdir -p /new/data /new/queue
                 cp -a /old-data/. /new/data/
                 if [ -d /old-queue ] && [ -n "$(ls -A /old-queue)" ]; then
                   cp -a /old-queue/. /new/queue/
                 fi
             volumeMounts:
               - name: old-data
                 mountPath: /old-data
                 readOnly: true
               - name: old-queue
                 mountPath: /old-queue
                 readOnly: true
               - name: new-klio
                 mountPath: /new
         volumes:
           - name: old-data
             persistentVolumeClaim:
               claimName: data-<server-name>-klio-0
           - name: old-queue
             persistentVolumeClaim:
               claimName: queue-<server-name>-klio-0
           - name: new-klio
             persistentVolumeClaim:
               claimName: klio-<server-name>-klio-0
   ```

1. **Apply the new Server CR**: same name, updated image, new CRD shape, with
   `spec.storage` sized to match the old total size and StorageClass
   used across `data`, both caches, and `queue`. The StatefulSet adopts
   the PVC you created in step 2 by name, instead of provisioning
   an empty one.

1. **Confirm the Server is healthy** against the migrated data before removing
    the old PVCs. The old PVCs can be deleted after the new Server is
    running and healthy.
