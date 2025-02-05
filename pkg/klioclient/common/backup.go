package common

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TablespaceLayout is the on-disk structure of a tablespace.
type TablespaceLayout struct {
	// Name is the tablespace name
	Name string `json:"name"`

	// Oid is the OID of the tablespace.
	Oid string `json:"oid"`

	// Path is the path where the tablespace can be found.
	Path string `json:"path"`
}

// BackupExecutor guides the execution of a PostgreSQL backup, delegating
// the upload process to the underlying implementation.
type BackupExecutor struct {
	name string

	pgData      string
	tablespaces []TablespaceLayout

	options  BackupOptions
	startLSN uint64
	impl     BackupExecutorImplementation
}

// BackupMetadata is the metadata to be stored with set of backup snapshots.
type BackupMetadata struct {
	Name          string `json:"name"`
	StartLSN      uint64 `json:"startLsn"`
	EndLSN        uint64 `json:"endLsn"`
	BackupLabel   string `json:"backupLabel"`
	TablespaceMap string `json:"tablespaceMap"`
}

// BackupExecutorImplementation is used by a backup executor to upload
// pgdata.
type BackupExecutorImplementation interface {
	UploadTablespace(ctx context.Context, tbl TablespaceLayout) error
	UploadPgData(ctx context.Context, pgData string) error
	FinishBackup(ctx context.Context, metadata BackupMetadata) error
}

// BackupOptions are the information needed to take a backup.
type BackupOptions struct {
	// A connection to the target database
	Connection *pgx.Conn

	// Progress is the callback to be used to report the backup status
	Progress Progress
}

// BackupClient is the interface that wraps the backend PGDATA storage.
type BackupClient interface {
	// CreateBackup takes a backup of the passed PostgreSQL layout.
	// Returns the eventual error status or an ID of the taken snapshot
	CreateBackup(ctx context.Context, options BackupOptions) (*BackupExecutor, error)
}

// Start starts the execution of a backup.
func (b *BackupExecutor) Start(ctx context.Context) error {
	b.name = time.Now().Format("20060102150405")

	row := b.options.Connection.QueryRow(ctx, "SHOW data_directory")
	if err := row.Scan(&b.pgData); err != nil {
		return fmt.Errorf("while reading pgdata: %w", err)
	}

	//nolint:godox
	// TODO(leonardoce): how should we discover the tablespaces?
	// perhaps it would be better to look inside PGData and
	// read the links
	tablespaceRows, err := b.options.Connection.Query(
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

	row = b.options.Connection.QueryRow(
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
		if err := b.impl.UploadTablespace(ctx, tbl); err != nil {
			return err //nolint:wrapcheck
		}
	}

	if err := b.impl.UploadPgData(ctx, b.pgData); err != nil {
		return err //nolint:wrapcheck
	}

	return nil
}

// Close finishes a backup.
func (b *BackupExecutor) Close(ctx context.Context) error {
	row := b.options.Connection.QueryRow(ctx, "SELECT lsn - '0/0', labelfile, spcmapfile FROM pg_backup_stop()")

	var (
		endLSN     uint64
		labelFile  []byte
		spcmapFile []byte
	)

	if err := row.Scan(&endLSN, &labelFile, &spcmapFile); err != nil {
		return fmt.Errorf("while running pg_backup_stop: %w", err)
	}

	//nolint:wrapcheck
	return b.impl.FinishBackup(ctx, BackupMetadata{
		Name:          b.name,
		StartLSN:      b.startLSN,
		EndLSN:        b.startLSN,
		BackupLabel:   string(labelFile),
		TablespaceMap: string(spcmapFile),
	})
}

// NewBackupExecutorForImpl creates a new backup executor for the passed implementation.
func NewBackupExecutorForImpl(impl BackupExecutorImplementation, opts BackupOptions) *BackupExecutor {
	return &BackupExecutor{
		impl:    impl,
		options: opts,
	}
}
