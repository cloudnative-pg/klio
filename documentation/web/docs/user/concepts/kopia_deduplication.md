---
sidebar_position: 4
description: >-
  How Kopia stores base backups as content-addressed chunks.
---

# Data deduplication in Klio

Klio stores base backups in a [Kopia](https://kopia.io/) repository.

Consecutive base backups of the same PostgreSQL cluster can share
a lot of content. Between two runs relation files may be untouched,
and the files that did change can have changed only for a small part.
Keeping a full independent copy of every backup would multiply all of
that unchanged data by the number of backups you retain.

Kopia avoids it by storing each distinct piece of data once and
referencing it from every backup that contains it.

## Content-addressable storage

When Klio uploads a base backup, Kopia splits the data into chunks,
which it calls *contents*. Each content is identified by a hash of its
own bytes — its *content ID*. Two chunks holding identical bytes
therefore get an identical content ID, whichever file or backup they came
from, so the repository only ever needs one copy of those bytes.

![Three base backups laid out as rows, with one column per distinct
content: chunks in the same column share a content ID, so ten chunk
references across the three backups resolve to five stored contents,
each backup adding only what changed](../images/kopia-deduplication.svg)

Because a chunk's identity comes from its contents rather than from its
filename or its position in a file, this works across backups without
Klio having to track what changed. Untouched relations produce the
same content IDs as the one before it, and Kopia recognizes that it
already holds them.

Sharing is not limited to consecutive pairs, either. A content introduced
by one backup is reused by every later backup that still contains those
bytes, so the cost of a backup depends on how much data changed since the
last one rather than on how many backups came before.

## What is deduplicated, and what is not

Deduplication applies to **base backups only**.

WAL files take a different path. They are not stored in the Kopia
repository at all: Klio compresses and encrypts each segment and writes
it to the WAL archive as an individual file. The size of the WAL archive
is therefore driven by how much WAL your cluster produces and how long
you keep it, not by how similar consecutive segments happen to be.

## Sharing is repository-wide

A content is shared across everything in the same repository, not only
across the backups of a single cluster. Each PostgreSQL cluster appears
in the repository as its own Kopia *host*, but all hosts draw on one
shared pool of contents. Where several clusters hold similar data,
their base backups reinforce each other's deduplication.

Tier 1 and Tier 2 are separate repositories, so the sharing happens
independently within each of them. See
[Architectures &amp; Tiers](architectures.md).

## Why deleting a backup may not free space

Because contents are shared, deleting a backup does not necessarily
delete any data. A content has to stay for as long as *any* retained
backup still references it, so removing one backup out of many may free
nothing at all. The space is reclaimed later, by Kopia maintenance, once
no backup points at a content any more.

![The same three backups with backup 1 removed: contents A, B, D and E
are still referenced by backups 2 and 3 and so remain in the repository,
while C, which only backup 1 referenced, is no longer referenced by
anything and can be reclaimed](../images/kopia-backup-deletion.svg)

For the full mechanism and what to do about a full disk, see
[How Disk Space Is Freed](../managing_storage.md#how-disk-space-is-freed).