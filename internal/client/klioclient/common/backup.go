package common

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/jackc/pgx/v5"
)

// controlDataPath is the name of the pg_controldata file.
const controlDataPath = "global/pg_control"

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
	pgData      string
	startLSN    uint64
	tablespaces []TablespaceLayout

	// A connection to the target database
	Connection *pgx.Conn

	uploader BackupUploader
}

// BackupMetadata is the metadata to be stored with set of backup snapshots.
type BackupMetadata struct {
	// Name is the backup name
	Name string `json:"name"`

	// StartLSN is the LSN of the backup start
	StartLSN uint64 `json:"startLsn"`

	// EndLSN is the LSN of the backup end
	EndLSN uint64 `json:"endLsn"`

	// BackupLabel is the backup label content
	BackupLabel string `json:"backupLabel"`

	// TablespaceMap is the tablespace map content
	TablespaceMap string `json:"tablespaceMap"`

	// Tablespaces are the metadata of the tablespaces
	Tablespaces []TablespaceLayout `json:"tablespaces"`

	// Annotations is a generic data store where each
	// backend can put its metadata.
	Annotations map[string]string `json:"annotations"`
}

// BackupUploader is used by a backup executor to upload several backup data.
type BackupUploader interface {
	// UploadTablespace uploads the tablespace with the passed layout to
	// the backup store.
	UploadTablespace(ctx context.Context, tbl TablespaceLayout) error

	// UploadPgData uploads the PGData to the backup store.
	UploadPgData(ctx context.Context, pgData string) error

	// UploadControlFile uploads the control file to the backup store.
	UploadControlFile(ctx context.Context, controlDataFileName string) error

	// UploadBackupMetadata is called to mark a backup successfully done.
	UploadBackupMetadata(ctx context.Context, metadata BackupMetadata) error
}

// NewBackupExecutor creates a new backup executor for the passed implementation.
func NewBackupExecutor(conn *pgx.Conn, uploader BackupUploader) *BackupExecutor {
	return &BackupExecutor{
		uploader:   uploader,
		Connection: conn,
	}
}

// Start starts the execution of a backup.
func (b *BackupExecutor) Start(ctx context.Context) error {
	b.name = time.Now().Format("20060102150405")

	row := b.Connection.QueryRow(ctx, "SHOW data_directory")
	if err := row.Scan(&b.pgData); err != nil {
		return fmt.Errorf("while reading pgdata: %w", err)
	}

	//nolint:godox
	// TODO(leonardoce): how should we discover the tablespaces?
	// perhaps it would be better to look inside PGData and
	// read the links
	tablespaceRows, err := b.Connection.Query(
		ctx,
		`SELECT oid::text, spcname::text, pg_tablespace_location(oid)::text FROM pg_tablespace`,
	)
	if err != nil {
		return fmt.Errorf("while reading list of tablespaces: %w", err)
	}

	for tablespaceRows.Next() {
		tbl := TablespaceLayout{}

		if err := tablespaceRows.Scan(&tbl.Oid, &tbl.Name, &tbl.Path); err != nil {
			return fmt.Errorf("while reading list of tablespaces (scan): %w", err)
		}

		if len(tbl.Path) == 0 {
			continue
		}

		b.tablespaces = append(b.tablespaces, tbl)
	}
	tablespaceRows.Close()

	if err := tablespaceRows.Err(); err != nil {
		return fmt.Errorf("while reading tablespaces: %w", err)
	}

	row = b.Connection.QueryRow(
		ctx,
		"SELECT pg_backup_start($1, fast:=true) - '0/0'",
		b.name,
	)
	if err := row.Scan(&b.startLSN); err != nil {
		return fmt.Errorf("while running pg_backup_start: %w", err)
	}

	return nil
}

// Upload starts the uploading process.
func (b *BackupExecutor) Upload(ctx context.Context) error {
	for _, tbl := range b.tablespaces {
		if err := b.uploader.UploadTablespace(ctx, tbl); err != nil {
			return err //nolint:wrapcheck
		}
	}

	if err := b.uploader.UploadPgData(ctx, b.pgData); err != nil {
		return err //nolint:wrapcheck
	}

	controlDataFileName := path.Join(b.pgData, controlDataPath)
	if err := b.uploader.UploadControlFile(ctx, controlDataFileName); err != nil {
		return err //nolint:wrapcheck
	}

	return nil
}

// Close finishes a backup.
func (b *BackupExecutor) Close(ctx context.Context) error {
	row := b.Connection.QueryRow(ctx, "SELECT lsn - '0/0', labelfile, spcmapfile FROM pg_backup_stop()")

	var (
		endLSN     uint64
		labelFile  []byte
		spcmapFile []byte
	)

	if err := row.Scan(&endLSN, &labelFile, &spcmapFile); err != nil {
		return fmt.Errorf("while running pg_backup_stop: %w", err)
	}

	//nolint:wrapcheck
	return b.uploader.UploadBackupMetadata(ctx, BackupMetadata{
		Name:          b.name,
		StartLSN:      b.startLSN,
		EndLSN:        b.startLSN,
		BackupLabel:   string(labelFile),
		TablespaceMap: string(spcmapFile),
	})
}
