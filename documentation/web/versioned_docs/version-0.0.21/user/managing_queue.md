---
title: Managing the Work Queue
sidebar_position: 9.5
---

# Managing the Work Queue

This guide explains how to monitor the Klio server's work queue and how to
handle tasks that failed too many attempts. For what the queue is and where
it is stored, see [the Klio server reference](klio_server.md#the-work-queue);
this page is about operating it.

## How the Queue Works

The queue is backed by NATS JetStream with file storage on the server's PVC,
so queued tasks survive a server restart. It runs two task streams, one per
job type:

- **WAL archiving** — for every WAL segment or partial file a PostgreSQL
  instance streams to the server, the server enqueues one task that reads
  the segment from its Tier 1 archive and copies it to Tier 2.
- **Backup processing** — when a base backup completes, the server enqueues
  one task that verifies the cluster's Tier 1 backups server-side and, when
  the backup requested Tier 2 relay, migrates the cluster's backups to
  Tier 2 and verifies them there.

Each stream is consumed one task at a time: a task is retried automatically
after a failure, with the delay growing between attempts until they are
exhausted:

| Task type | Attempts before dead-letter | Retry delays |
|-----------|-----------------------------|--------------|
| WAL archiving | 10 | 30 s, 1 min, 2 min, 5 min |
| Backup processing | 5 | 5 min, 10 min, 30 min, 1 h |

A running task signals progress to the queue, so a slow WAL archiving or
backup verification is not redelivered just because it takes long. When the
attempts are exhausted, the queue stops delivering the task and files a
dead-letter queue entry for it — see below.

## Monitoring the Queue

`klio admin queue status` reports how many backup and WAL tasks are pending
on the Klio server:

```bash
kubectl exec -it klio-server-klio-0 -- klio admin queue status
```

You can also watch queue activity through the server metrics — WAL archiving
durations, backup relays and verifications are exposed there; see
[the metrics reference](opentelemetry.md).

## Failed Tasks and the Dead-Letter Queue

A queued task that exhausts its attempts (retry schedule above) is moved to
a dead-letter queue: the queue stops delivering it, and nothing retries it
automatically again — an operator must retry or discard it. List failed
tasks with `klio admin queue wal list-failed` and
`klio admin queue backup list-failed`.

If the failure was transient — for example, the Tier 2 object store was
unreachable — re-enqueue the failed tasks with `klio admin queue wal retry`
and `klio admin queue backup retry`. Re-enqueued tasks are delivered like
new ones, with a fresh attempt budget, and when a retried task succeeds the
server removes its dead-letter entries on its own.

If a task should never run again, remove it with
`klio admin queue wal discard` or `klio admin queue backup discard`.
Discarding is destructive and permanent: a discarded task is never re-run.

- A discarded backup task skips its post-backup processing. The base backup
  is safe in Tier 1, but it stays without server-side verification until the
  next successful backup of the same cluster (which verifies and relays all
  of the cluster's pending backups). Until then, the only copy of that
  backup is on the Klio server's Tier 1 storage.
- A discarded WAL task is never archived to Tier 2. The WAL segment stays in
  the local Tier 1 archive — until Tier 1 retention removes it — but it is
  missing from Tier 2, so a recovery served by a read-only (Tier 2-only)
  server cannot cross the gap: point-in-time recovery to a target after a
  discarded WAL is impossible from Tier 2 alone.
