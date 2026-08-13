# Kopia Internals

This document explains Kopia's internal architecture, storage systems, and core mechanisms like sessions and checkpoints.

## Table of Contents

1. [Storage Architecture](#storage-architecture)
2. [Sessions: The Write Protection Mechanism](#sessions-the-write-protection-mechanism)
3. [Checkpoints: Periodic Data Protection](#checkpoints-periodic-data-protection)

---

## Storage Architecture

Kopia uses two complementary storage systems:

1. **Content-Addressable Storage**: For backup data (files, directories)
2. **Label-Addressable Storage**: For metadata (snapshots, policies)

### Content-Addressable Storage

Backup data is stored using content-addressable storage:

1. **Contents**: Fixed-size chunks of data, identified by their hash (content ID)
2. **Pack Blobs**: Multiple contents are grouped into pack blobs for efficient storage
3. **Index Blobs**: Map content IDs to their location within pack blobs
4. **Objects**: Higher-level structures (files, directories) composed of multiple contents

### Pack Blob Types: Segregation by Content Type

Pack blobs are segregated based on the type of content they contain. The content ID prefix determines which pack type is used:

```go
// From content_manager.go
func packPrefixForContentID(contentID ID) blob.ID {
    if contentID.HasPrefix() {
        return PackBlobIDPrefixSpecial  // "q" packs
    }
    return PackBlobIDPrefixRegular      // "p" packs
}
```

| Pack Prefix | Blob Name Pattern | Content Type | Content ID Examples |
|-------------|-------------------|--------------|---------------------|
| `p` (regular) | `p7f8a9b0-session1` | File data chunks | `abc123def...` (no prefix) |
| `q` (special) | `q3c4d5e6-session2` | Metadata | `mabc123...` (manifests), `kabc123...` (other) |

**Content ID prefixes**:
- No prefix: Raw file data (stored in `p` packs)
- `m`: Manifest contents (stored in `q` packs)
- `k`: Other metadata like policies (stored in `q` packs)
- `n`: Index blob contents (stored separately, not in pack blobs)

**Why segregation matters**:
- Different caching strategies: Data (`p`) and metadata (`q`) can be cached separately
- Different access patterns: Metadata is accessed frequently, data less so
- GC behavior: System contents in `q` packs (manifests, indexes) are never garbage collected by Snapshot GC

### Index Blobs: The Content Location Map

Index blobs are critical for understanding how Kopia finds data and determines what's "referenced":

**What an index entry contains**:

```go
// From repo/content/index/info.go
type Info struct {
    ContentID           ID        // Hash-based identifier (e.g., "abc123def...")
    PackBlobID          blob.ID   // Which pack blob contains this content
    PackOffset          uint32    // Byte offset within the pack blob
    PackedLength        uint32    // Size in bytes (after compression/encryption)
    OriginalLength      uint32    // Original size before compression
    Deleted             bool      // Soft-delete flag for GC
    TimestampSeconds    int64     // When this content was written
    CompressionHeaderID           // Compression algorithm used
    // ...
}
```

**How index blobs are loaded and searched**:

Kopia doesn't know which index blob contains a specific content ID. Instead, it loads **ALL index blobs** into memory and merges them into a single searchable structure:

```go
// From repo/content/index/merged.go
type Merged []Index  // Slice of all loaded index blobs

// GetInfo searches ALL indexes for a content ID
func (m Merged) GetInfo(id ID, result *Info) (bool, error) {
    for _, ndx := range m {
        ok, err := ndx.GetInfo(id, &tmp)
        if ok {
            // Found! Keep the one with highest timestamp
            // (handles updates/deletions across multiple indexes)
        }
    }
}
```

**How content lookup works**:

```
┌─────────────────────────────────────────────────────────────┐
│                Content Lookup Process                       │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. On repository open: Load ALL index blobs                │
│     Index blobs: n0a1b2c3, n4d5e6f7, n8g9h0i1, ...         │
│     Merge into single searchable structure                  │
│                                                             │
│  2. Application requests content by ContentID               │
│     ContentID = "abc123def456..."                           │
│                                                             │
│  3. Search merged index for ContentID                       │
│     (searches across all loaded index blobs)                │
│     Found: PackBlobID="p7f8a9b0-session1"                   │
│            PackOffset=1024                                  │
│            PackedLength=4096                                │
│                                                             │
│  4. Read from pack blob at specified location               │
│     Read 4096 bytes from "p7f8a9b0-session1" at offset 1024 │
│                                                             │
│  5. Decrypt and decompress to get original content          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Why are there multiple index blobs?**

Each `Flush()` operation writes a new index blob containing entries for the contents written since the last flush. Over time, this creates many small index blobs. Index compaction (part of maintenance) merges these into larger blobs for efficiency.

**Index blobs determine which packs are "referenced"**:

A pack blob is considered **referenced** if ANY index entry points to it via `PackBlobID`. This is how Pack GC decides what to keep:

```go
// Simplified from content_manager_iterate.go
func IterateUnreferencedPacks() {
    // Step 1: Build set of all PackBlobIDs from all index entries
    usedPacks := NewSet()
    for each indexEntry in allIndexes:
        usedPacks.Add(indexEntry.PackBlobID)

    // Step 2: Any pack blob NOT in usedPacks is "unreferenced"
    for each packBlob in storage:
        if not usedPacks.Contains(packBlob.ID):
            // This pack can potentially be deleted
            callback(packBlob)
}
```

**Key insight**: A pack blob becomes "referenced" when an index blob containing its ID is written. Before the index is written, the pack exists in storage but nothing points to it - it's "unreferenced" and vulnerable to Pack GC. This is precisely why sessions exist.

### Label-Addressable Storage (Manifests)

Metadata is stored in **manifests** - JSON documents identified by labels rather than content hash:

1. **Snapshot Manifests**: Record what was backed up, when, and the root object ID
2. **Policy Manifests**: Define retention rules, compression settings, etc.
3. **Other Manifests**: Maintenance schedules, ACLs, etc.

Each manifest has:
- A unique ID
- A set of labels (key-value pairs) including a required `type` label
- A modification timestamp
- The actual JSON payload

```
┌─────────────────────────────────────────────────────────────┐
│                    Storage Hierarchy                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  LABEL-ADDRESSABLE (Manifests)                              │
│  ─────────────────────────────                              │
│  Snapshot Manifest                                          │
│    ├─ Labels: {type: "snapshot", hostname: "...", ...}      │
│    ├─ RootObjectID: "Iabcdef123..."                         │
│    └─ StartTime, EndTime, Stats, etc.                       │
│                                                             │
│           │                                                 │
│           ▼                                                 │
│                                                             │
│  CONTENT-ADDRESSABLE (Data)                                 │
│  ─────────────────────────                                  │
│  Root Object (directory manifest)                           │
│    └─ References child objects by Object ID                 │
│           │                                                 │
│           ▼                                                 │
│  Child Objects (files, subdirectories)                      │
│    └─ Files are composed of Content IDs                     │
│           │                                                 │
│           ▼                                                 │
│  Contents (stored in Pack Blobs)                            │
│    └─ Located via Index Blobs                               │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### GC Roots: What Keeps Data Alive

Kopia has **two levels of garbage collection**, each with different "roots":

**Level 1 - Snapshot GC (Content-Level)**:
- **GC Roots**: Snapshot manifests
- **What's protected**: Contents reachable from any snapshot
- **What gets marked deleted**: Contents not reachable from any snapshot

**Level 2 - Pack GC (Blob-Level)**:
- **GC Roots**: Index blobs (via PackBlobID references)
- **What's protected**: Pack blobs referenced by any index entry
- **What gets deleted**: Pack blobs with no index entries pointing to them

**Snapshot GC process** (marks contents as deleted):

1. **Enumerate all snapshot manifests** (from label-addressable storage)
2. **Walk the object tree** starting from each snapshot's root object ID
3. **Mark all reachable content IDs** as "in use"
4. **Any content not marked** is eligible for deletion

```go
// From snapshotgc/gc.go - simplified
func findInUseContentIDs(ctx context.Context, rep repo.Repository, used *Set) error {
    // Step 1: Get all snapshot manifests (GC roots)
    manifests, _ := snapshot.LoadSnapshots(ctx, rep, ids)

    // Step 2: Walk each snapshot's object tree
    for _, m := range manifests {
        root := snapshotfs.SnapshotRoot(rep, m)
        // Walk tree, marking all content IDs as used
        walker.Process(ctx, root, "")
    }
}
```

**Important implications**:
- Deleting a snapshot manifest makes its unique data eligible for GC
- Data shared between snapshots remains alive as long as ANY snapshot references it
- Manifests themselves are stored as contents with a special prefix (`m`)
- System contents (manifests, indexes) are never garbage collected

### The Two-Phase Write Problem

When writing data, there's a critical window between:
1. Writing pack blobs to storage
2. Writing the index that references those packs

During this window, the pack blobs are **unreferenced** - they exist in storage but no index points to them. If garbage collection runs during this window, it might delete these packs, corrupting the in-progress backup.

This is the fundamental problem that sessions solve.

---

## Sessions: The Write Protection Mechanism

### What is a Session?

A session is Kopia's mechanism for protecting in-progress writes from garbage collection. When a write operation begins, Kopia:

1. Generates a unique **Session ID**
2. Writes a **Session Marker Blob** to storage
3. Embeds the Session ID in all pack blob names created during the session

### Session Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                    Session Lifecycle                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. SESSION START (on first write)                          │
│     ├─ Generate unique Session ID                           │
│     ├─ Write Session Marker blob ONCE (prefix: "s")         │
│     │   └─ CheckpointTime set to NOW                        │
│     └─ Session Marker contains:                             │
│         - ID                                                │
│         - StartTime                                         │
│         - CheckpointTime (= StartTime, never updated!)      │
│         - User, Host                                        │
│                                                             │
│  2. DATA WRITING                                            │
│     ├─ Pack blobs created with Session ID in name           │
│     │   Format: {prefix}{randomID}-{sessionID}              │
│     ├─ Packs are UNREFERENCED (not in any index yet)        │
│     └─ Session Marker is NOT updated during this phase      │
│                                                             │
│  3. SESSION COMMIT (during Flush)                           │
│     ├─ Write Index Blobs (references all packs)             │
│     ├─ DELETE Session Marker blobs (not update - delete!)   │
│     └─ Packs are now REFERENCED (protected by index)        │
│                                                             │
│  Note: After commit, next write starts a NEW session        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Pack Blob Naming Convention

Pack blobs include the session ID in their name:

```
p{random64bit}-{sessionID}    # Regular pack blob
q{random64bit}-{sessionID}    # Special pack blob (for indexes, etc.)
```

This allows garbage collection to identify which session created each pack and whether that session is still active.

### Session Marker Structure

Session markers are stored as blobs with the `s` prefix and contain JSON metadata:

```json
{
  "id": "abc123-epoch5",
  "startTime": "2024-01-15T10:00:00Z",
  "checkpointTime": "2024-01-15T10:00:00Z",
  "username": "backup-user",
  "hostname": "backup-server"
}
```

The `checkpointTime` field is critical - it indicates when the session was last known to be active.

### Critical: Session Markers Are Immutable

**The session marker is written ONCE when the session starts and is NEVER updated during the session's lifetime.**

From `sessions.go`:

```go
func (bm *WriteManager) writeSessionMarkerLocked(ctx context.Context) error {
    cp := bm.currentSessionInfo
    cp.CheckpointTime = bm.timeNow()  // Set at write time, never updated
    // ... write to storage ...
}

// TODO(jkowalski): write this periodically when sessions span the duration of an upload.
```

This means:
- `CheckpointTime` equals the session start time
- If a session runs for hours without committing, the CheckpointTime becomes stale
- After `SessionExpirationAge` (96 hours), the session appears "expired" to maintenance

**This is why checkpoints are essential**: They don't update the session marker - they **commit the current session and start a new one** with a fresh CheckpointTime.

### How Sessions Protect Blobs

During garbage collection, when evaluating whether to delete an unreferenced pack:

```go
// From pack_gc.go
sid := content.SessionIDFromBlobID(bm.BlobID)
if s, ok := activeSessions[sid]; ok {
    if age := cutoffTime.Sub(s.CheckpointTime); age < safety.SessionExpirationAge {
        // PRESERVE - pack belongs to an active session
        return nil
    }
}
// DELETE - pack is truly orphaned
```

The pack is preserved if:
1. Its session ID matches an active session marker, AND
2. The session's CheckpointTime is within `SessionExpirationAge`

---

## Checkpoints: Periodic Data Protection

### What Are Kopia Checkpoints?

Kopia checkpoints are periodic save points during long-running backup operations. They serve two purposes:

1. **Progress Saving**: Allow resuming interrupted backups
2. **Session Renewal**: Commit current session's data and start a fresh session

**Important distinction**: Checkpoints do NOT update the existing session marker. Instead, they:
1. Write index blobs (making current packs referenced/protected)
2. Delete the current session marker (commit)
3. Let the next write operation create a NEW session with a fresh CheckpointTime

This is why checkpoints protect long-running backups despite session markers being immutable.

### The 45-Minute Checkpoint Interval

By default, Kopia creates a checkpoint every 45 minutes during snapshot uploads:

```go
// From upload.go
const DefaultCheckpointInterval = 45 * time.Minute
```

This interval is enforced as a maximum - you cannot set a longer interval:

```go
if u.CheckpointInterval > DefaultCheckpointInterval {
    return nil, errors.Errorf("checkpoint interval cannot be greater than %v", DefaultCheckpointInterval)
}
```

### What Happens During a Checkpoint

When a checkpoint triggers, the following sequence occurs:

```
┌─────────────────────────────────────────────────────────────┐
│                   Checkpoint Sequence                       │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. CHECKPOINT TRIGGERED (every 45 minutes)                 │
│                                                             │
│  2. Flush() CALLED                                          │
│     ├─ finishAllPacksLocked()                               │
│     │   └─ Complete any pending pack writes                 │
│     │                                                       │
│     └─ flushPackIndexesLocked()                             │
│         ├─ Write Index Blobs                                │
│         │   └─ All packs now REFERENCED (protected!)        │
│         └─ commitSession()                                  │
│             └─ DELETE Session Marker blobs                  │
│                 (session S1 is now finished)                │
│                                                             │
│  3. NEW SESSION STARTS (on next write)                      │
│     └─ getOrStartSessionLocked() creates NEW session S2    │
│         └─ NEW Session Marker written                       │
│             └─ CheckpointTime = NOW (fresh timestamp!)      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Key insight**: The old session marker is deleted, not updated. Protection for the old session's packs now comes from the index, not the session marker.

### Checkpoints Protect Long-Running Backups

For a 2-week database backup:

```
Hour 0.00:  Session S1 starts (CheckpointTime = Hour 0)
            Packs P1-P100 written (UNREFERENCED, protected by S1 marker)

Hour 0.75:  CHECKPOINT
            ├─ Index I1 written → P1-P100 now REFERENCED by index
            ├─ Session S1 marker DELETED (committed)
            └─ P1-P100 no longer need session protection (index protects them)

Hour 0.76:  First write after checkpoint
            └─ Session S2 starts (CheckpointTime = Hour 0.76)
            Packs P101-P200 written (UNREFERENCED, protected by S2 marker)

Hour 1.50:  CHECKPOINT
            ├─ Index I2 written → P101-P200 REFERENCED
            ├─ Session S2 marker DELETED
            └─ Session S3 will start on next write
...
Hour 336:   Final flush, backup complete
```

**Key insight**: Even though the backup takes 2 weeks, at any given time only the most recent 45 minutes of data is unreferenced. All earlier data has been indexed and is protected by the index, not by session markers.

**The 96-hour SessionExpirationAge is a safety margin**, not a limit on backup duration. It protects against scenarios where checkpoints fail repeatedly.
