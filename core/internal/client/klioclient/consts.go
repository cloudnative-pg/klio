package klioclient

// BackupNameTagName is the name of the tag containing the
// backup name.
const BackupNameTagName = "klio.io/tag"

// BackupContentTagName is the name of the tag containing the
// snapshot content.
const BackupContentTagName = "klio.io/content"

// TablespaceNameTagName is the name of the tag containing the
// name of the tablespace.
const TablespaceNameTagName = "klio.io/tablespaceName"

// Tier2Pin is the name of the pin indicating that this
// snapshot should not be deleted until it is uploaded to tier2.
const Tier2Pin = "klio.io/tier2"
