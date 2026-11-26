---
sidebar_position: 9.7
description: >-
  How to deploy the Klio Server in production so that a pod, node, zone,
  volume, or site failure loses as little data as possible.
---

# Running Klio in Production

This page describes the recommended way to deploy Klio in production, and
what each recommendation protects you from.

## The Availability Model

A Klio Server is a single pod with a single volume. Klio does not run
replicated servers. Instead:

- **Availability** comes from restarting the server quickly on the same
  volume, on any node.
- **Durability** comes from that volume and from Tier 2.

The design tolerates a server outage. While the server is down:

- **No WAL is lost.** The WAL streamer uses the `klio` physical replication
  slot, which only advances once the Klio Server confirms that the data is
  durable. PostgreSQL keeps the WAL files that the server has not received,
  and the streamer reconnects on its own, within about 20 seconds from the
  moment the server is back. The cost is that WAL files build up on the
  PostgreSQL volume for as long as the outage lasts.
- **Backups fail.** A backup that starts or is running during the outage
  fails and is not resumed. The next scheduled backup runs normally.
- **Restores are not available**, including restores from Tier 2 through
  that server: the same pod serves both tiers.

The goal of a production deployment is therefore to keep outages short, and to
make sure that losing the server's volume does not mean losing the backups.

## Failure Scenarios

| Failure | What happens | What you need |
|---------|--------------|---------------|
| Server pod restarts | Short outage. WAL streaming resumes, queued tasks survive. | Nothing more. |
| Node fails | The pod moves to another node only if its volume can follow it. | [Network storage](#storage-for-the-klio-server). |
| Zone fails | The pod stays down if its volume belongs to the lost zone. | Zone-replicated storage, or wait for the zone. |
| Tier 1 volume is lost | Everything that had not reached Tier 2 is lost. | [Tier 2](#tier-2), and a [new server](#after-a-complete-tier-1-loss). |
| Tier 2 is unreachable | Tier 1 keeps working. The relay queue grows. | [Queue sizing](klio_server.md#queue-sizing) and monitoring. |
| Site is lost | Tier 1 is gone with the Kubernetes cluster. | A [recovery site](#disaster-recovery-site) on the same Tier 2. |

## Storage for the Klio Server

The storage class of the server's PersistentVolumeClaim (PVC) is the most
important choice in the deployment.

### Use Network Storage

Use storage that Kubernetes can detach from a failed node and attach to
another one. With such storage, a node failure is only an outage: the
server restarts elsewhere with all of its data.

Do not use node-local storage, such as `hostPath` volumes, `local`
volumes, or volume groups on the node's own disks. The pod cannot
start on any other node, and if the node does not come back, Tier 1 is
lost.

:::note
When a node stops responding, Kubernetes does not release a `ReadWriteOnce`
volume by itself, and the server pod stays in the `Terminating` state. See
[Non-graceful node shutdown](https://kubernetes.io/docs/concepts/cluster-administration/node-shutdown/#non-graceful-node-shutdown)
in the Kubernetes documentation for how to let the pod move.
:::

### Consider the Zone

Many network volumes belong to a single availability zone. The server pod
can then only run on nodes of that zone. If the zone fails, the server is
down until the zone is back, and PostgreSQL keeps the WAL files in the
meantime.

If this is not acceptable, use storage that is replicated across zones.
Otherwise, make sure that the PostgreSQL volumes can hold the WAL files of
a zone outage (see [PostgreSQL Side](#postgresql-side)).

### Keep It Separate from PostgreSQL

Use a storage backend that does not share a failure with the PostgreSQL
volumes. A backup stored on the same disks as the database does not survive
the loss of those disks. See
[Tier 1 Architectures](concepts/architectures.md#tier-1-architectures).

### Storage Class Settings

- `allowVolumeExpansion: true`, so that a full volume can be resized in
  place. See [Managing Storage](managing_storage.md).
- `reclaimPolicy: Retain`, so that deleting the PVC by mistake does not
  delete the data.

## Scheduling

Run the Klio Server on different nodes from the PostgreSQL instances it
protects, so that one node failure does not take down both the primary and
its backup server. Use `.spec.template` in the `Server` resource for this.
See
[Reserving Nodes for Klio Workloads](concepts/architectures.md#reserving-nodes-for-klio-workloads)
and
[Node Affinity and Tolerations](klio_server.md#node-affinity-and-tolerations).

In the same template, set a `priorityClassName` high enough that the
scheduler does not evict the server to make room for other workloads.

## PostgreSQL Side

During a server outage, PostgreSQL keeps every WAL file that Klio has not
received. Size the PostgreSQL WAL storage for the longest outage you want
to survive, and monitor its usage.

Be careful with the `max_slot_wal_keep_size` PostgreSQL parameter. A limit
protects the primary from a full disk, but when the limit is reached,
PostgreSQL invalidates the `klio` slot and removes WAL files that Klio never
received. The WAL archive then has a gap, and you need a new base backup to
be able to recover past it.

### Synchronous Replication

Synchronous replication is managed by PostgreSQL, not by Klio. The WAL
streamer connects as a standby named `klio` and reports its write and flush
positions correctly, so PostgreSQL can use it as a synchronous standby.

In CloudNativePG, add `klio` to `synchronous_standby_names` with the
`standbyNamesPre` or `standbyNamesPost` field of the cluster's
[synchronous replication](https://cloudnative-pg.io/docs/current/replication/#synchronous-replication)
settings:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
spec:
  postgresql:
    synchronous:
      method: any
      number: 1
      standbyNamesPre:
        - klio
```

`standbyNamesPre` puts `klio` before the PostgreSQL instances of the
cluster, and `standbyNamesPost` puts it after them. The position only
matters with the priority-based method (`first`), where the first names in
the list are the synchronous standbys.

With synchronous replication, every confirmed transaction is on at least
`number` standbys, and `klio` counts as one of them. What happens during a
Klio Server outage depends on the other standbys:

- If the other standbys are enough to reach `number`, writes continue.
  Every confirmed transaction is on one of those standbys, and Klio
  receives the missing WAL from the `klio` slot when the server is back.
- If PostgreSQL cannot reach `number` synchronous standbys without `klio`,
  commits wait while the Klio Server is down.

## Tier 2

Tier 1 is a single volume. Tier 2 is the only protection against losing
it, so every production server needs one.

- Configure `tier2` on the `Server`, on object storage that does not share
  a failure with the Kubernetes cluster.
- In every `PluginConfiguration`, set `tier2.enableBackup: true`,
  `tier2.enableRecovery: true`, and a `tier2.retention` policy. See
  [Tier 2 configuration](plugin_configuration.md#tier-2-configuration).

Klio relays data to Tier 2 asynchronously. At any moment, the following
exists only on Tier 1:

- The partial WAL segment being streamed.
- The WAL files that are waiting in the work queue, or that are in the
  dead-letter queue.
- The base backups that are waiting for the next relay.

Keep this set small: watch the queue and act on failed tasks. See
[Managing the Work Queue](managing_queue.md).

### After a Complete Tier 1 Loss

A lost Tier 1 cannot be recreated. To recover:

1. Create a new `Server` that uses the same Tier 2: the same bucket and
   prefix, and the same encryption key.
1. Point the PostgreSQL clusters to the new server.
1. Take a new base backup of every cluster.

Everything that had not reached Tier 2 is gone. The WAL archive has a gap
between the last WAL file on Tier 2 and the new base backup, and
point-in-time recovery is not possible inside that gap.

:::warning
The new server can read Tier 2 only with the original encryption key. Keep
a copy of the encryption key and of the Age identity outside the Kubernetes
cluster. See [Encryption](klio_server.md#encryption).
:::

## Disaster Recovery Site

To recover in another region or data center, deploy a
[read-only server](klio_server.md#read-only-mode) there, on the same Tier 2
bucket. It does not need a copy of Tier 1, and several read-only servers can
use the same bucket.

The recovery site needs the encryption key, the Age identity, and the
certificate authority of the client certificates. Make them available
there before you need them.

## Separating Tenants

All clusters that back up to one Klio Server share its volume, its
encryption key, and its outages. Use separate servers for clusters that
belong to different tenants. See
[Tier 1 Architectures](concepts/architectures.md#tier-1-architectures).
