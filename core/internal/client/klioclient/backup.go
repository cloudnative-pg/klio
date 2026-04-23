package klioclient

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/jackc/pgx/v5"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// controlDataPath is the name of the pg_controldata file.
const controlDataPath = "global/pg_control"

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

	uploader Client

	startedAt int64
}

// NewBackupExecutor creates a new backup executor for the passed implementation.
func NewBackupExecutor(conn *pgx.Conn, uploader Client, clusterName string) *BackupExecutor {
	return &BackupExecutor{
		tablespaces: nil,
		Connection:  conn,
		uploader:    uploader,
		clusterName: clusterName,
	}
}

// BackupOptions contain the backup options.
type BackupOptions struct {
	// Name is the backup name. If not set a new name will be generated
	// using the current timestamp.
	Name string
}

// Start starts the execution of a backup.
func (b *BackupExecutor) Start(ctx context.Context, opts BackupOptions) error {
	now := time.Now()
	b.startedAt = now.Unix()

	b.name = now.Format("20060102150405")
	if opts.Name != "" {
		b.name = opts.Name
	}

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
func (b *BackupExecutor) Upload(ctx context.Context, pinned bool) error {
	contextLogger := log.FromContext(ctx)

	for i, tbl := range b.tablespaces {
		contextLogger.Info("Uploading tablespace",
			"tablespace", tbl.Name,
			"oid", tbl.Oid,
			"current", i+1,
			"total", len(b.tablespaces),
		)
		if err := b.uploader.UploadTablespace(ctx, b.name, tbl, pinned); err != nil {
			return err //nolint:wrapcheck
		}
	}

	contextLogger.Info("Uploading PGDATA")
	if err := b.uploader.UploadPgData(ctx, b.name, b.pgData, pinned); err != nil {
		return err //nolint:wrapcheck
	}

	contextLogger.Info("Uploading control file")
	controlDataFileName := path.Join(b.pgData, controlDataPath)
	if err := b.uploader.UploadControlFile(ctx, b.name, controlDataFileName, pinned); err != nil {
		return err //nolint:wrapcheck
	}

	return nil
}

// Close finishes a backup.
func (b *BackupExecutor) Close(ctx context.Context, pinned bool) (*BackupMetadata, error) {
	contextLogger := log.FromContext(ctx)

	var tli int
	var segmentSize uint64

	tliRow := b.Connection.QueryRow(ctx, "SELECT timeline_id FROM pg_control_checkpoint()")
	if err := tliRow.Scan(&tli); err != nil {
		contextLogger.Error(err, "While getting current timeline, skipped")
	}

	segmentRow := b.Connection.QueryRow(ctx, "SELECT bytes_per_wal_segment FROM pg_control_init()")
	if err := segmentRow.Scan(&segmentSize); err != nil {
		contextLogger.Error(err, "While getting segment size, skipped")
	}

	row := b.Connection.QueryRow(ctx, "SELECT lsn - '0/0', labelfile, spcmapfile FROM pg_backup_stop()")

	var (
		endLSN     uint64
		labelFile  []byte
		spcmapFile []byte
	)

	if err := row.Scan(&endLSN, &labelFile, &spcmapFile); err != nil {
		return nil, fmt.Errorf("while running pg_backup_stop: %w", err)
	}

	//nolint:wrapcheck
	metadata := &BackupMetadata{
		Name:          b.name,
		ClusterName:   b.clusterName,
		StartedAt:     b.startedAt,
		StoppedAt:     time.Now().Unix(),
		StartLSN:      b.startLSN,
		EndLSN:        endLSN,
		BackupLabel:   string(labelFile),
		TablespaceMap: string(spcmapFile),
		Tablespaces:   b.tablespaces,
		Timeline:      tli,
		SegmentSize:   segmentSize,
		PgData:        b.pgData,
	}

	if tli > 0 && segmentSize > 0 {
		startWALFile, err := types.Int64ToLSN(b.startLSN).WALFileName(tli, segmentSize)
		if err != nil {
			contextLogger.Error(err, "While computing the WAL name when the backup started")
		} else {
			metadata.StartWAL = startWALFile
		}

		endWALFile, err := types.Int64ToLSN(endLSN).WALFileName(tli, segmentSize)
		if err != nil {
			contextLogger.Error(err, "While computing the WAL name when the backup started")
		} else {
			metadata.EndWAL = endWALFile
		}
	}

	contextLogger.Info("Uploading backup metadata")
	if err := b.uploader.UploadBackupMetadata(ctx, b.name, metadata, pinned); err != nil {
		return nil, fmt.Errorf("while uploading backup metadata: %w", err)
	}

	return metadata, nil
}
