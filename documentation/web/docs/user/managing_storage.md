---
sidebar_position: 9
---

# Managing Storage

This guide explains how to manage storage on your Klio server, prevent
disk full scenarios, and recover when storage is exhausted.

The Klio server uses a single PersistentVolumeClaim (PVC), mounted at
`/klio`, for backup data, WAL archives, Kopia caches, and the work
queue — see [Storage Requirements](klio_server.md#storage-requirements)
for the full layout. Because everything shares one volume, growth in
any of these areas eats into the space the others need. When the PVC
approaches capacity, backup and WAL archival operations may fail.

## How Disk Space Is Freed

Deleting a backup does **not** immediately free disk space. Klio is built
on top of [Kopia](https://kopia.io), which uses a two-phase approach:

1. **Deletion** logically deletes backups
1. **Maintenance** actually removes deleted data, if the data is older
   than 24 hours and unused by any existing backup

This design prevents accidental data loss from concurrent operations.
However, it means that space is not freed until maintenance runs and the
24-hour safety window has passed.

Maintenance runs automatically in the background. Klio does not currently
provide a way to trigger it on demand, but it can be started manually
by opening a shell on the Klio server pod and running:

```bash
kopia maintenance set --owner=me \
  --config-file=/tmp/$KOPIACONFIG_TIER1_CONF \
  --disable-file-logging
kopia maintenance run \
  --config-file=/tmp/$KOPIACONFIG_TIER1_CONF \
  --full \
  --disable-file-logging
```

## When the Disk Is Full

When the Klio server's `/klio` PVC is completely full:

- All backup operations block (new backups, deletions, maintenance)
- WAL streaming to Klio stops
- PostgreSQL accumulates WAL files on its own PVC
- No backup corruption occurs, even when backups fail mid-operation
- No orphan or incomplete snapshots are left behind
- Existing backups remain intact and restorable

All operations resume automatically when space is freed.

:::warning
While the Klio disk is full, WAL files build up on the PostgreSQL PVC.
If this condition persists, it can lead to disk pressure on the database
side as well. Resolve Klio storage issues promptly to avoid cascading
failures.
:::

## Resolving Storage Issues

### Expand the PVC

The simplest option is to expand the PVC. The Klio operator supports
expansion of its PVC.

#### Prerequisites

PVC expansion requires a StorageClass with `allowVolumeExpansion: true`.

:::warning User Responsibility
Before attempting to resize PVCs, **you must verify** that your
StorageClass supports volume expansion. The operator will attempt the
resize operation directly—if the StorageClass does not support
expansion, the Kubernetes API will reject the request and the operator
will log an error.
:::

To check if your StorageClass supports expansion:

```bash
kubectl get storageclass <your-storage-class> -o jsonpath='{.allowVolumeExpansion}'
```

If the output is not `true`, you need to either:
1. Update the StorageClass to enable volume expansion (if the underlying
   storage provisioner supports it)
1. Use a different StorageClass that supports volume expansion
1. Migrate to a new PVC (see [Limitations](#limitations) for options)

#### Expanding PVC Size

To expand the PVC, update the
`spec.storage.pvcTemplate.resources.requests.storage` field in the
Server spec with a larger value:

```yaml
apiVersion: klio.cnpg.io/v1alpha1
kind: Server
metadata:
  name: klio-server
spec:
  storage:
    pvcTemplate:
      resources:
        requests:
          storage: 220Gi  # Increased from 120Gi
```

Apply the updated Server resource:

```bash
kubectl apply -f klio-server.yaml
```

#### What Happens During Resize

When you update the Server spec with a larger PVC size, the following
occurs:

1. **PVC expansion**: The operator patches the `klio` PVC directly to
   the new size. This modifies the PVC resources but does **not**
   update the StatefulSet—the StatefulSet's VolumeClaimTemplates remain
   unchanged at this point.
1. **Temporary misalignment**: After the PVC patch, there is a brief
   period where the PVC has the new size but the StatefulSet
   VolumeClaimTemplates still reflect the old size.
1. **StatefulSet recreation**: The operator detects that the expected
   StatefulSet (with new VolumeClaimTemplates) differs from the current
   one. Since VolumeClaimTemplates are immutable in Kubernetes, the
   StatefulSet is deleted and recreated to align with the new spec.
1. **Pod restart**: The Klio server pod restarts and mounts the
   already-expanded PVC.

:::note Why explicit PVC patching is necessary
VolumeClaimTemplates only define specs for *new* PVCs—they do not resize
existing ones. Without explicit PVC patching by the operator, the
StatefulSet would be recreated but the PVC would remain at its
original size, creating a permanent mismatch between the Server spec
and actual storage.
:::

#### StatefulSet and PVC Alignment

After the full resize operation completes:

- The **PVC** has the new expanded size
- The **StatefulSet VolumeClaimTemplates** match the new size (after
  recreation)
- The **Server spec** is consistent with both

This ensures no drift between the desired state and actual resources.

#### Monitoring Resize Progress

The operator emits a `PVCExpanded` Kubernetes event on the Server
resource when the PVC is successfully expanded. You can view these events
with:

```bash
kubectl describe server klio-server
```

Check the PVC status to monitor the resize operation. Since there is
only one PVC per server, the label selector returns exactly one
result:

```bash
kubectl get pvc -l klio.cnpg.io/klio-server=klio-server
```

The PVC will show the new requested size in
`spec.resources.requests.storage`. The actual capacity is reflected in
`status.capacity.storage` once the resize completes.

For detailed status, including any resize conditions:

```bash
kubectl describe pvc klio-klio-server-klio-0
```

:::note PVC naming
The PVC name follows Kubernetes' StatefulSet convention
`<volumeClaimTemplateName>-<statefulSetName>-<ordinal>`. The
volume claim template is itself named `klio`, and the StatefulSet is
named `<server-name>-klio`, so for a server named `klio-server` the PVC
is `klio-klio-server-klio-0` — the doubled `klio` is expected, not a
typo.
:::

#### Limitations

- **Expansion only**: PVC shrinking is not supported by Kubernetes.
  Attempting to decrease `spec.storage.pvcTemplate.resources.requests.storage`
  is rejected by the API server at admission time, with the error
  `storage PVC size cannot be decreased`.
- **StorageClass support**: The StorageClass must have
  `allowVolumeExpansion: true`. If the StorageClass does not support
  expansion, the resize will fail and an error will be logged.
- **Pod restart required**: Due to StatefulSet VolumeClaimTemplates
  being immutable, PVC expansion causes a brief pod restart.
- **Filesystem resize**: After the volume is expanded, the filesystem
  must also be resized. Most modern storage providers handle this
  automatically.

:::warning No Automatic Fallback
If your StorageClass does not support volume expansion, there is **no
automatic fallback**. The operator will not delete and recreate PVCs to
achieve a larger size, as this would result in **permanent data loss**.
The only options in this case are:

1. Migrate to a StorageClass that supports volume expansion
1. Create a new Klio server with a larger PVC and restore from backup
1. Manually migrate data to a new PVC on the same StorageClass (requires
   downtime and careful planning — see [Migrating from the Multi-PVC
   Model](upgrade_notes.md#migrating-from-the-multi-pvc-model) for the
   closely related procedure used when moving off an old, per-component
   PVC layout)

:::

### Delete Backups and Run Maintenance

:::warning
Maintenance requires some free disk space to run. If the disk is
completely full, maintenance itself may fail. In that case, expand
the PVC first.
:::

If PVC expansion is not available, free space by deleting old backups
and running maintenance manually.

1. **Delete old backups:**

   ```bash
   kubectl exec -it klio-server-klio-0 -- \
     klio admin delete-backup <oldest-backup-name> \
       --cluster cluster-example --tier1
   ```

   To delete from both tiers, add `--tier2`.

   :::note
   The actual space freed depends on how much data is shared with
   other backups through deduplication. Deleting a backup only
   reclaims space for data blocks that are not referenced by any
   remaining backup.
   :::

   :::warning
   Deleting a backup removes the ability to restore to that point
   in time. Ensure you have adequate backups remaining before
   deletion.
   :::

1. **Run maintenance** to reclaim space from deleted backups by opening
   a shell on the Klio server pod and running:

   ```bash
   kopia maintenance set --owner=me \
     --config-file=/tmp/$KOPIACONFIG_TIER1_CONF \
     --disable-file-logging
   kopia maintenance run \
     --config-file=/tmp/$KOPIACONFIG_TIER1_CONF \
     --full \
     --disable-file-logging
   ```

## Best Practices

1. **Configure retention policies**: The most effective way to control
   storage growth is through properly configured retention policies, which
   automatically delete old backups and WAL files no longer needed for
   recovery. See
   [Retention Policies](plugin_configuration.md#retention-policies) for
   configuration details.

1. **Monitor storage usage**: Klio does not provide built-in storage
   alerts. Set up monitoring and alerting on your PVC usage to detect
   capacity issues before they cause failures.

1. **Size the PVC appropriately**: Account for your backup
   frequency, database size, change rate, and retention requirements
   when provisioning it, and add headroom for the Kopia caches and the
   work queue on top of that — see
   [Storage Requirements](klio_server.md#storage-requirements). Include
   buffer for the 24-hour window during which deleted backup data is
   not yet eligible for garbage collection.

1. **Use Tier 2 for long-term retention**: Object storage (S3, etc.) is
   more cost-effective and scales easily for long-term backup retention.
   Keep Tier 1 lean for fast recovery of recent backups.

1. **Use expandable StorageClasses**: When possible, use StorageClasses
   with `allowVolumeExpansion: true` to enable online PVC expansion as a
   recovery option.