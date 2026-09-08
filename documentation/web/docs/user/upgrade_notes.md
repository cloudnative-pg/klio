---
sidebar_position: 91
---

# Upgrade Notes

This page lists version-specific changes that may require
manual action when upgrading Klio. For the upgrade procedure,
see the [Helm chart page](helm_chart.mdx#upgrades).

## 0.0.20 to 0.0.21

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
Upgrading the operator to 0.0.21 deletes the StatefulSet for
**every** Server still on the old PVC layout, the
moment the new operator starts reconciling them — not when you get
around to migrating a given server. There is no way to defer or stage
this per server. Work out the PVC name and StorageClass for each
server (step 2 below) and schedule a maintenance window *before*
upgrading the operator, not after.
:::

1. **Installing the operator 0.0.21 will delete the StatefulSets and
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

   :::warning Verify this name against your cluster
   Getting this name wrong means the StatefulSet provisions a brand
   new, empty PVC instead of adopting the one you pre-created, and the
   copied data is silently orphaned on a PVC nothing mounts. Confirm
   the exact name a StatefulSet named `<server-name>-klio` will look
   for before relying on this procedure.
   :::

   For a server named `<server-name>`, the PVC manifest looks like:

   ```yaml
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: klio-<server-name>-klio-0
     labels:
       klio.cnpg.io/klio-server: <server-name>
       klio.cnpg.io/pvcType: klio
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
