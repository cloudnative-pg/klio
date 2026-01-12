package kopia

import "fmt"

// Manifest represents information about a single point-in-time filesystem snapshot.
type Manifest struct {
	// ID is the unique identifier of the snapshot.
	ID string `json:"id"`

	// Source contains information about what was snapshotted.
	Source SourceInfo `json:"source"`

	// RootEntry is the root directory entry of the snapshot.
	RootEntry *DirEntry `json:"rootEntry"`

	// Description is a user-provided description of the snapshot.
	Description string `json:"description"`

	// StartTime is when the snapshot started.
	StartTime string `json:"startTime"`

	// EndTime is when the snapshot completed.
	EndTime UTCTimestamp `json:"endTime"`

	// IncompleteReason describes why the snapshot is incomplete, if applicable.
	IncompleteReason string `json:"incomplete,omitempty"`

	// RetentionReasons lists the reasons why this snapshot is being retained.
	RetentionReasons []string `json:"-"`

	// Tags contains user-defined key-value pairs associated with the snapshot.
	Tags map[string]string `json:"tags,omitempty"`

	// Pins is a list of manually-defined pins which prevent the snapshot from being deleted.
	Pins []string `json:"pins,omitempty"`
}

// DirEntry represents a directory entry as stored in JSON stream.
type DirEntry struct {
	// Name is the name of the file or directory.
	Name string `json:"name,omitempty"`

	// FileSize is the size of the file in bytes.
	FileSize int64 `json:"size,omitempty"`

	// ModTime is the last modification time of the entry.
	ModTime UTCTimestamp `json:"mtime,omitempty"`

	// UserID is the numeric user ID of the file owner.
	UserID uint32 `json:"uid,omitempty"`

	// GroupID is the numeric group ID of the file owner.
	GroupID uint32 `json:"gid,omitempty"`

	// DirSummary contains summary information for directories.
	DirSummary *DirectorySummary `json:"summ,omitempty"`
}

// DirectorySummary represents summary information about a directory.
type DirectorySummary struct {
	// TotalFileSize is the total size of all files in the directory tree in bytes.
	TotalFileSize int64 `json:"size"`

	// TotalFileCount is the total number of files in the directory tree.
	TotalFileCount int64 `json:"files"`

	// TotalSymlinkCount is the total number of symbolic links in the directory tree.
	TotalSymlinkCount int64 `json:"symlinks"`

	// TotalDirCount is the total number of subdirectories in the directory tree.
	TotalDirCount int64 `json:"dirs"`

	// MaxModTime is the latest modification time found in the directory tree.
	MaxModTime UTCTimestamp `json:"maxTime"`

	// IncompleteReason describes why the directory summary is incomplete, if applicable.
	IncompleteReason string `json:"incomplete,omitempty"`

	// FatalErrorCount is the number of failed files.
	FatalErrorCount int `json:"numFailed"`

	// IgnoredErrorCount is the number of errors that were ignored.
	IgnoredErrorCount int `json:"numIgnoredErrors,omitempty"`
}

// SourceInfo represents the coordinates of what is being snapshotted.
type SourceInfo struct {
	// Host is the hostname where the snapshot was taken.
	Host string `json:"host"`

	// UserName is the username that took the snapshot.
	UserName string `json:"userName"`

	// Path is the filesystem path that was snapshotted.
	Path string `json:"path"`
}

func (ssi SourceInfo) String() string {
	if ssi.Host == "" && ssi.Path == "" && ssi.UserName == "" {
		return "(global)"
	}

	if ssi.Path == "" {
		return fmt.Sprintf("%v@%v", ssi.UserName, ssi.Host)
	}

	return fmt.Sprintf("%v@%v:%v", ssi.UserName, ssi.Host, ssi.Path)
}

// Policy describes snapshot policy for a single source.
type Policy struct {
	// Labels contains key-value pairs associated with this policy.
	Labels map[string]string `json:"-"`

	// RetentionPolicy defines how long snapshots should be retained.
	RetentionPolicy RetentionPolicy `json:"retention"`

	// NoParent indicates whether this policy inherits from parent policies.
	NoParent bool `json:"noParent,omitempty"`
}

// RetentionPolicy describes snapshot retention policy.
type RetentionPolicy struct {
	// KeepLatest is the number of most recent snapshots to keep.
	KeepLatest *int `json:"keepLatest,omitempty"`

	// KeepHourly is the number of hourly snapshots to keep.
	KeepHourly *int `json:"keepHourly,omitempty"`

	// KeepDaily is the number of daily snapshots to keep.
	KeepDaily *int `json:"keepDaily,omitempty"`

	// KeepWeekly is the number of weekly snapshots to keep.
	KeepWeekly *int `json:"keepWeekly,omitempty"`

	// KeepMonthly is the number of monthly snapshots to keep.
	KeepMonthly *int `json:"keepMonthly,omitempty"`

	// KeepAnnual is the number of annual snapshots to keep.
	KeepAnnual *int `json:"keepAnnual,omitempty"`
}

// Target is used to point a Kopia transaction to the set of snapshots
// having the specified Hostname and Username.
type Target struct {
	// Hostname is the hostname of the snapshot, as in the
	// <username>@<hostname> snapshot indicator.
	Hostname string

	// Username is the name of the user that took the snapshot, as in the
	// <username>@<hostname> snapshot indicator.
	Username string
}

// String formats this target as the string that the Kopia CLI
// would expect.
func (t Target) String() string {
	return fmt.Sprintf("%s@%s", t.Username, t.Hostname)
}
