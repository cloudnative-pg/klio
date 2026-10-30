---
sidebar_position: 91
toc_max_heading_level: 4
---

# Upgrade Notes

This page lists version-specific changes that may require
manual action when upgrading Klio. For the generic upgrade procedure,
see the [Helm chart page](helm_chart.mdx#upgrades).

## 0.0.20 to 0.0.21

Version 0.0.21 introduces several breaking changes to the `Server` and
`PluginConfiguration` CRDs, and a new unified storage layout for each `Server`.
There is no automatic conversion: every `Server` must be migrated by hand, and
Klio cannot archive WAL files or take backups from the CRD upgrade until each
`Server` is migrated. Plan a maintenance window.

The procedure below has four phases, and only the first one is free of
downtime. Follow the steps in order: each step contains everything you need to
perform it.

| Change | What you do | Step |
|---|---|---|
| Klio server ports changed | Update network policies and firewall rules | [1](#step-1-check-the-prerequisites) |
| Credentials are file references | Rewrite the `Server` and `PluginConfiguration` | [2](#step-2-rewrite-the-credentials) |
| Klio-managed retention policies | Replace the `keep*` fields with `latest` | [3](#step-3-update-the-retention-policies) |
| The `klio-restore` container is gone | Remove it from `PluginConfiguration` | [4](#step-4-remove-the-klio-restore-container) |
| Unified storage layout | Create a new PVC and copy the data | [5](#step-5-prepare-the-unified-storage), [9–12](#phase-3-migrate-each-server-downtime-continues) |
| Grafana dashboard requires Grafana 13.1 | Import the new dashboard | [After the upgrade](#after-the-upgrade-grafana-dashboard) |

### Phase 1: Prepare (no downtime)

Do all of this before the maintenance window. Steps 2 to 6 edit copies of your
manifests, which you apply in steps 11 and 13.

#### Step 1: Check the prerequisites

1. Save a copy of each `Server` and `PluginConfiguration` manifest. The next
   steps update these copies to the new API.

1. Make sure the PostgreSQL volumes have room for the WAL files that pile up
   in the `klio` replication slot while Klio is down.

1. The Klio server now listens on ports outside the Linux ephemeral port
   range, so that an outgoing connection can no longer occupy one of them
   before the server binds it:

   | Port              | Before  | Now     |
   |-------------------|---------|---------|
   | Tier 1 base       | `51515` | `11515` |
   | Tier 1 WAL        | `52000` | `12000` |
   | Tier 2 base       | `51516` | `11516` |
   | Tier 2 WAL        | `52001` | `12001` |

   The operator updates the Server's StatefulSet and Service, and the client
   configuration of every `PluginConfiguration`, automatically. Update any
   NetworkPolicy, firewall rule or manually written client configuration (for
   example a WAL player Job) that references the old ports.

#### Step 2: Rewrite the credentials

Every credential used to be the name of a Secret. It is now a file reference:
a `volume` (a `secret`, `configMap`, `projected` or `csi` source) plus the
path of the file inside it. You can keep using the Secrets you created for
0.0.20: nothing needs to be re-issued or copied, you only point the new fields
at the same Secrets.

| Resource | Before (0.0.20) | After (0.0.21) |
|---|---|---|
| `Server` | `tlsSecretName` | `tls.serverIdentity` |
| `Server` | `caSecretName` | `tls.clientCa` |
| `Server` | `tier{1,2}.encryptionKeyFile` and `tier{1,2}.identityFile` | `tier{1,2}.encryption` |
| `Server` | `tier2.s3.customCaBundle` (`name`, `key`) | `tier2.s3.customCaBundle` (`volume`, `path`) |
| `PluginConfiguration` | `clientSecretName` | `tls.clientIdentity` |
| `PluginConfiguration` | `serverSecretName` | `tls.serverCa` |

The S3 `accessKeyId` and `secretAccessKey` references are unchanged.
`credentialsFile` and `profile` are now available as a mutually exclusive
alternative to the S3 static keys.

`certPath` and `keyPath` name the files inside the volume, so they are not
tied to the fixed `tls.crt`, `tls.key` and `ca.crt`. Those names are still the
right paths when the source stays a plain Secret. To mount only the CA out of
a Secret that also holds a private key, add `items` mapping the CA key, as in
the examples below.

**Rewrite each `Server` manifest:**

- Replace `tlsSecretName` with `tls.serverIdentity`. The Secret is the
  same, and the paths are `tls.crt` and `tls.key` when it was created
  by cert-manager.
- Replace `caSecretName` with `tls.clientCa`, using the `ca.crt` key
  of the same Secret.
- Merge `encryptionKeyFile` and `identityFile` into one `encryption`
  block per tier. Both files come from a single volume, so use the
  Secret that held them, and set `keyPath` and `identityPath` to the
  old `path` values.
- If you set `tier2.s3.customCaBundle`, replace the `name` and `key` pair
  with a `volume` and a `path` (see the second example below).

```yaml
spec:
  tls:
    serverIdentity:
      volume:
        secret:
          secretName: <previous tlsSecretName>
      certPath: tls.crt
      keyPath: tls.key
    clientCa:
      volume:
        secret:
          secretName: <previous caSecretName>
          items:
            - key: ca.crt
              path: ca.crt
      path: ca.crt
  tier1:
    encryption:
      volume:
        secret:
          secretName: <Secret that held the encryption key and identity>
      keyPath: <previous encryptionKeyFile path>
      identityPath: <previous identityFile path>
```

For a Tier 2 with a custom CA bundle for the S3 endpoint, the old
`name` is the Secret and the old `key` is the file inside it. Use the
Secret as the volume, and map the old `key` to a file with `items`:

```yaml
# Before (0.0.20)
tier2:
  s3:
    customCaBundle:
      name: rustfs-tls
      key: ca.crt
```

```yaml
# After (0.0.21)
tier2:
  s3:
    customCaBundle:
      volume:
        secret:
          secretName: rustfs-tls
          items:
            - key: ca.crt
              path: ca.crt
      path: ca.crt
```

When the old `key` is already `ca.crt`, the `items` list is optional:
set `path: ca.crt` and the volume alone.

**Rewrite each `PluginConfiguration` manifest:**

- Replace `clientSecretName` with `tls.clientIdentity`.
- Replace `serverSecretName` with `tls.serverCa`. Clients now verify the
  server through a CA bundle instead of pinning its certificate, so use
  the `ca.crt` key of that Secret.

```yaml
spec:
  tls:
    clientIdentity:
      volume:
        secret:
          secretName: <previous clientSecretName>
      certPath: tls.crt
      keyPath: tls.key
    serverCa:
      volume:
        secret:
          secretName: <previous serverSecretName>
          items:
            - key: ca.crt
              path: ca.crt
      path: ca.crt
```

#### Step 3: Update the retention policies

Retention is now enforced by the Klio server, which periodically evaluates
each cluster's backups against its policy and deletes the ones that fall
outside it, instead of delegating the decision to Kopia.

In each `PluginConfiguration`, the `retention` block changed shape. The
Kopia-style `keepLatest`, `keepHourly`, `keepDaily`, `keepWeekly`,
`keepMonthly` and `keepAnnual` fields are replaced by a single `latest`
field, which keeps the given number of most recent backups. Update any
`tier1.retention` and `tier2.retention` blocks accordingly. Omit the block,
or set `latest` to `0`, to keep every backup.

```yaml
# Before (0.0.20)
tier1:
  retention:
    keepLatest: 10
    keepHourly: 48
    keepDaily: 7
    keepWeekly: 4
    keepMonthly: 24
    keepAnnual: 1
tier2:
  enableBackup: true
  retention:
    keepLatest: 10
    keepAnnual: 2
```

```yaml
# After (0.0.21)
tier1:
  retention:
    latest: 10
tier2:
  enableBackup: true
  retention:
    latest: 10
```

The time-based fields have no equivalent: only the number of most recent
backups is kept, so choose `latest` for each tier on its own.

`klio retention set` no longer accepts the `--keep-*` flags.

**Incomplete backups are no longer deleted automatically.** Retention used
to delete the backups that were not fully present on a tier, such as
interrupted uploads, along with their leftover snapshots. It now only
evaluates complete backups and never deletes an incomplete one: they are
reported instead, and removed by hand. Incomplete backups already on the
Klio server are no longer cleaned up, so their snapshots take space on the
PVC until deleted.

List them with `klio admin list-backups --incomplete` and delete the ones
that can never complete with `klio admin delete-backup`. See
[Incomplete Backups](backup_and_restore.md#incomplete-backups).

#### Step 4: Remove the `klio-restore` container

The `klio-plugin` sidecar now also serves the restore while a cluster
bootstraps from a Klio backup, so Klio no longer injects a separate
`klio-restore` container into the recovery Job. The `PluginConfiguration`
CRD only accepts `klio-plugin` in `spec.containers`: a `PluginConfiguration`
that still lists a `klio-restore` entry is rejected on its next change to
`spec.containers`.

If you have it, remove the `klio-restore` entry from each
`PluginConfiguration` and move any customization you need onto `klio-plugin`.

#### Step 5: Prepare the unified storage

Klio servers created until v0.0.20 used four separate PersistentVolumeClaims
per server: `data`, `cachetier1`, `cachetier2`, and `queue`. The `Server` CRD
no longer accepts that shape: `spec.storage.pvcTemplate` is mandatory, and the
old `tier1.data`, `tier1.cache`, `tier2.cache`, and top-level `queue` fields
are no longer valid.

Only `data` (the actual backups and WAL) and `queue` (pending,
not-yet-processed tasks) contain information that should be moved to the new
PVC. `cachetier1` and `cachetier2` are recreated and populated on demand.

For each `Server`:

1. **Update the copy of the `Server` manifest.** Remove `tier1.data`,
   `tier1.cache`, `tier2.cache` and `queue`, and add `spec.storage`, sized to
   the old total across `data`, both caches and `queue` combined, with the
   same StorageClass:

   ```yaml
   spec:
     storage:
       pvcTemplate:
         storageClassName: <same StorageClass as the old PVCs>
         accessModes:
           - ReadWriteOnce
         resources:
           requests:
             storage: <old data + cachetier1 + cachetier2 + queue sizes>
   ```

1. **Create a PVC named to match what the StatefulSet will
   adopt.** Following the StatefulSet PVC naming convention
   `<volumeClaimTemplateName>-<statefulSetName>-<ordinal>`, where the
   unified volume claim template is named `klio` and the StatefulSet is
   named `<server-name>-klio`, the PVC to create is
   `klio-<server-name>-klio-0` (for example, `klio-klio-server-klio-0`
   for a server named `klio-server`). Give it the same size as
   `spec.storage` in the new manifest.

   The operator hardcodes this name: the StatefulSet always looks for
   `klio-<server-name>-klio-0`.

   :::warning Getting this name wrong orphans the copied data
   The StatefulSet provisions a brand new, empty PVC instead of
   adopting the one you pre-created, and the copied data is silently
   orphaned on a PVC nothing mounts.
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

   You can create it with `kubectl apply -f <pvc-manifest>.yaml`.

:::note Read-only servers
A read-only `Server` has only the `cachetier2` PVC, and a cache holds nothing
to copy. For it, update the manifest as above, but skip the PVC creation here
and [step 10](#step-10-copy-the-data): the operator creates the PVC of the new
`Server`, and you delete the old PVC once it is running.
:::

#### Step 6: Set the Server image

Set `spec.image` of each `Server` manifest to the 0.0.21 Klio image,
`ghcr.io/cloudnative-pg/klio:v0.0.21`. The image of the `klio-plugin`
sidecar comes from the operator chart, so it needs no manual step.

### Phase 2: Upgrade the CRDs and the operator (downtime starts)

#### Step 7: Upgrade the CRDs

Follow [Upgrade the CRDs](helm_chart.mdx#upgrade-the-crds).

#### Step 8: Upgrade the operator

Follow [Upgrade the operator](helm_chart.mdx#upgrade-the-operator).

The PostgreSQL instance pods of every cluster using Klio are restarted to
roll out the new `klio-plugin` sidecar. They won't be able to reach the old
servers, and the operator will be unable to reconcile `Server` resources,
since they are on the old API.

### Phase 3: Migrate each Server (downtime continues)

Repeat steps 9 to 12 for each `Server`.

#### Step 9: Stop the old Klio server

Its StatefulSet keeps running, with the old PVCs mounted, until you apply the
new `Server` manifest. Scale it down before copying the data:

```shell
kubectl scale statefulset <server-name>-klio --replicas=0
```

The operator does not scale it back up while the `Server` still has the old
shape.

#### Step 10: Copy the data

Run a one-off Job. Mount the old `data` PVC, and the old `queue` PVC if
present, read-only, alongside the new PVC (read-write), and copy:

- the old `data` PVC's contents into the new PVC's `data/`
- the old `queue` PVC's contents into the new PVC's `queue/`

For a server named `<server-name>`, following the old PVC naming
convention `<volumeClaimTemplateName>-<statefulSetName>-<ordinal>`
(`data-<server-name>-klio-0`, `queue-<server-name>-klio-0`), and the new
PVC created in [step 5](#step-5-prepare-the-unified-storage)
(`klio-<server-name>-klio-0`):

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

Wait for the Job to complete before the next step.

#### Step 11: Apply the new Server manifest

Apply the manifest you prepared in steps 2 to 6. The operator deletes the old
StatefulSet and creates a new one, which adopts the PVC you created by name,
instead of provisioning an empty one. The old PVCs survive this because the
StatefulSet's `persistentVolumeClaimRetentionPolicy` is `Retain`.

#### Step 12: Check that the Server is healthy

Confirm the Server is healthy against the migrated data before removing the
old PVCs. Check that the new StatefulSet adopted the PVC you pre-created.

The old PVCs can be deleted after the migration.

### Phase 4: Reconnect the clients and verify (downtime ends)

#### Step 13: Apply the PluginConfigurations

Apply each updated `PluginConfiguration`. The PostgreSQL instance pods are
restarted again to mount the credentials.

#### Step 14: Validation

Check the logs of the `klio-plugin` sidecar in each PostgreSQL instance pod
and in the Klio server. There should be no errors. Verify that the
Klio sidecars are able to stream the WALs.

### After the upgrade: Grafana dashboard

The Klio Grafana dashboard moved from
`observability/grafana/klio-dashboard.json` to
`observability/grafana/dashboards/klio.json`. It uses the dashboard schema
v2, with a Clusters and a Servers section that each have their own filters,
and needs Grafana 13.1 or later. See
[Grafana Dashboards](grafana-dashboards.md).

Import the new file in place of the previous dashboard. This does not require
downtime and can be done at any time.