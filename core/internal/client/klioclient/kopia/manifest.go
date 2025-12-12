package kopia

import "fmt"

// Manifest represents information about a single point-in-time filesystem snapshot.
type Manifest struct {
	ID     string     `json:"id"`
	Source SourceInfo `json:"source"`

	RootEntry *DirEntry `json:"rootEntry"`

	Description string       `json:"description"`
	StartTime   string       `json:"startTime"`
	EndTime     UTCTimestamp `json:"endTime"`

	IncompleteReason string `json:"incomplete,omitempty"`

	RetentionReasons []string `json:"-"`

	Tags map[string]string `json:"tags,omitempty"`

	// list of manually-defined pins which prevent the snapshot from being deleted.
	Pins []string `json:"pins,omitempty"`
}

// DirEntry represents a directory entry as stored in JSON stream.
type DirEntry struct {
	Name       string            `json:"name,omitempty"`
	FileSize   int64             `json:"size,omitempty"`
	ModTime    UTCTimestamp      `json:"mtime,omitempty"`
	UserID     uint32            `json:"uid,omitempty"`
	GroupID    uint32            `json:"gid,omitempty"`
	DirSummary *DirectorySummary `json:"summ,omitempty"`
}

// DirectorySummary represents summary information about a directory.
type DirectorySummary struct {
	TotalFileSize     int64        `json:"size"`
	TotalFileCount    int64        `json:"files"`
	TotalSymlinkCount int64        `json:"symlinks"`
	TotalDirCount     int64        `json:"dirs"`
	MaxModTime        UTCTimestamp `json:"maxTime"`
	IncompleteReason  string       `json:"incomplete,omitempty"`

	// number of failed files
	FatalErrorCount   int `json:"numFailed"`
	IgnoredErrorCount int `json:"numIgnoredErrors,omitempty"`
}

// SourceInfo represents the coordinates of what is being snapshotted.
type SourceInfo struct {
	Host     string `json:"host"`
	UserName string `json:"userName"`
	Path     string `json:"path"`
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
