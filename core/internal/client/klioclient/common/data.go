package common

import "github.com/jackc/pgx/v5"

// TablespaceLayout is the on-disk structure of a tablespace.
type TablespaceLayout struct {
	// Name is the tablespace name
	Name string `json:"name"`

	// Oid is the OID of the tablespace.
	Oid string `json:"oid"`

	// Path is the path where the tablespace can be found.
	Path string `json:"path"`

	// Annotations is a generic data store where each backend
	// can annotate its metadata.
	Annotations map[string]string `json:"annotations"`
}

// BackupExecutor guides the execution of a PostgreSQL backup, delegating
// the upload process to the underlying implementation.
type BackupExecutor struct {
	name        string
	clusterName string
	pgData      string
	startLSN    uint64
	tablespaces []TablespaceLayout

	// A connection to the target database
	Connection *pgx.Conn

	uploader BackupUploader

	startedAt int64
}

// BackupMetadata is the metadata to be stored with set of backup snapshots.
type BackupMetadata struct {
	// Name is the backup name
	Name string `json:"name"`

	// ClusterName is the name of the cluster that was backed up
	ClusterName string `json:"clusterName"`

	// StartLSN is the LSN of the backup start
	StartLSN uint64 `json:"startLsn"`

	// EndLSN is the LSN of the backup end
	EndLSN uint64 `json:"endLsn"`

	// StartWAL is the current WAL when the backup started
	StartWAL string `json:"startWal"`

	// EndWAL is the current WAL when the backup ends
	EndWAL string `json:"endWal"`

	// Timeline is the backup timeline
	Timeline int `json:"tli"`

	// SegmentSize is the segment size of the WALs during the backup.
	SegmentSize uint64 `json:"segmentSize"`

	// BackupLabel is the backup label content
	BackupLabel string `json:"backupLabel"`

	// TablespaceMap is the tablespace map content
	TablespaceMap string `json:"tablespaceMap"`

	// Tablespaces are the metadata of the tablespaces
	Tablespaces []TablespaceLayout `json:"tablespaces,omitempty"`

	// Annotations is a generic data store where each
	// backend can put its metadata.
	Annotations map[string]string `json:"annotations,omitempty"`

	// StartedAt is the current time when the backup started.
	StartedAt int64 `json:"startedAt"`

	// StoppedAt is the current time when the backup ended.
	StoppedAt int64 `json:"stoppedAt"`

	// Sources is the list of sources that have been uploaded by this backup.
	Sources []string `json:"sources"`
}

// BackupList is a list of backups.
type BackupList []BackupMetadata
