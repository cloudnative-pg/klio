package opentelemetry

import (
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
)

// Tracer name constants.
const (
	// TracerBackup is the tracer name for the plugin backup orchestration.
	TracerBackup = "klio.plugin.backup"
	// TracerConsumer is the tracer name for the server-side WAL consumer.
	TracerConsumer = "klio.server.consumer"
	// TracerWalServer is the tracer name for the server-side WAL ingest.
	TracerWalServer = "klio.server.wal"
	// TracerWalClient is the tracer name for the client-side WAL streaming.
	TracerWalClient = "klio.client.wal"
)

// Span name constants.
const (
	// DownloadHistoryFileSpan is the span name for downloading history files.
	DownloadHistoryFileSpan = "download_history_file"
	// ManageWalStreamSpan is the span name for managing WAL streams.
	ManageWalStreamSpan = "manage_wal_stream"
	// FlushBlockSpan is the span name for flushing blocks.
	FlushBlockSpan = "flush_block"
	// ReceiveBlockSpan is the span name for receiving blocks.
	ReceiveBlockSpan = "receive_block"
	// WriteBlockSpan is the span name for writing blocks.
	WriteBlockSpan = "write_block"
	// ReadBlockSpan is the span name for reading blocks.
	ReadBlockSpan = "read_block"
	// ReadBlockDataSpan is the span name for reading block data.
	ReadBlockDataSpan = "read_block_data"
	// WriteBlockDataSpan is the span name for writing block data.
	WriteBlockDataSpan = "write_block_data"
	// WrapBlockSpan is the span name for wrapping block data.
	WrapBlockSpan = "wrap_block_data"
	// UnwrapBlockSpan is the span name for unwrapping block data.
	UnwrapBlockSpan = "unwrap_block_data"
	// SendBlockSpan is the span name for sending blocks.
	SendBlockSpan = "send_block"
	// GetWalSpan is the span name for getting WAL files.
	GetWalSpan = "get_wal"
	// PutWalSpan is the span name for uploading WAL files.
	PutWalSpan = "put_wal"
	// BackupSpan is the span name for the backup entry point.
	BackupSpan = "backup"
	// BackupRunSpan is the span name for running a backup.
	BackupRunSpan = "backup_run"
	// BackupVerifySpan is the span name for verifying a backup.
	BackupVerifySpan = "backup_verify"
	// BackupMaintenanceSpan is the span name for running backup maintenance.
	BackupMaintenanceSpan = "backup_maintenance"
)

// Metric name constants.
const (
	PluginBackupLatestStartTimeMetric      = "klio.plugin.backup.latest_start_time"
	PluginBackupLatestCompletionTimeMetric = "klio.plugin.backup.latest_completion_time"
	PluginBackupLatestFailureTimeMetric    = "klio.plugin.backup.latest_failure_time"
	PluginBackupLatestDurationMetric       = "klio.plugin.backup.latest_duration"
	PluginBackupDurationMetric             = "klio.plugin.backup.duration"
	PluginBackupInProgressMetric           = "klio.plugin.backup.in_progress"
	PluginBackupRunsMetric                 = "klio.plugin.backup.runs"

	ServerWalWrittenSizeMetric           = "klio.server.wal.written_size"
	ServerWalWrittenMetric               = "klio.server.wal.written"
	ServerWalLatestWrittenTimeMetric     = "klio.server.wal.latest_written_time"
	ServerWalLatestWrittenLSNMetric      = "klio.server.wal.latest_written_lsn"
	ServerWalLatestWrittenTimelineMetric = "klio.server.wal.latest_written_timeline"

	ServerUptimeMetric                        = "klio.server.uptime"
	ServerBackupSnapshotsMetric               = "klio.server.backup.snapshots"
	ServerBackupLatestSnapshotSizeMetric      = "klio.server.backup.latest_snapshot_size"
	ServerBackupLatestSnapshotFilesMetric     = "klio.server.backup.latest_snapshot_files"
	ServerBackupLatestSnapshotDirsMetric      = "klio.server.backup.latest_snapshot_dirs"
	ServerBackupLatestSnapshotTimestampMetric = "klio.server.backup.latest_snapshot_timestamp"
	ServerBackupOldestSnapshotTimestampMetric = "klio.server.backup.oldest_snapshot_timestamp"
	ServerBackupVerificationsMetric           = "klio.server.backup.verifications"

	ServerBackupBackupsMetric               = "klio.server.backup.backups"
	ServerBackupLatestBackupStartTimeMetric = "klio.server.backup.latest_backup_start_time"
	ServerBackupLatestBackupEndTimeMetric   = "klio.server.backup.latest_backup_completion_time"
	ServerBackupLatestBackupStartLSNMetric  = "klio.server.backup.latest_backup_start_lsn"
	ServerBackupLatestBackupEndLSNMetric    = "klio.server.backup.latest_backup_end_lsn"
	ServerBackupLatestBackupTimelineMetric  = "klio.server.backup.latest_backup_timeline"
	ServerBackupOldestBackupStartTimeMetric = "klio.server.backup.oldest_backup_start_time"
	ServerBackupOldestBackupEndTimeMetric   = "klio.server.backup.oldest_backup_completion_time"
	ServerBackupOldestBackupStartLSNMetric  = "klio.server.backup.oldest_backup_start_lsn"
	ServerBackupOldestBackupEndLSNMetric    = "klio.server.backup.oldest_backup_end_lsn"
	ServerBackupOldestBackupTimelineMetric  = "klio.server.backup.oldest_backup_timeline"

	ServerQueueMessagesMetric = "klio.server.queue.messages"
	ServerQueueBytesMetric    = "klio.server.queue.bytes"
)

// PluginBackupMetrics holds OTel instruments for backup lifecycle tracking.
// The Runs counter carries an `outcome` attribute (`success` / `failure`)
// so a single instrument exposes both flavors. Runs failure data points
// additionally carry a `failure_category` attribute classifying the failure
// (see opentelemetry.FailureCategory); verification failures are recorded
// here with `failure_category="verification"`.
type PluginBackupMetrics struct {
	LatestStartTime      metric.Int64Gauge
	LatestCompletionTime metric.Int64Gauge
	LatestFailureTime    metric.Int64Gauge
	LatestDuration       metric.Float64Gauge
	Duration             metric.Float64Histogram
	InProgress           metric.Int64UpDownCounter
	Runs                 metric.Int64Counter
}

// ServerBackupMetrics holds OTel instruments for server-side backup state.
// It groups two related families:
//
//   - The Verifications counter, paired with `klio.plugin.backup.*` from the
//     plugin sidecar: the plugin records backup lifecycle, the server records
//     the verifications it runs against those backups. Each recording carries
//     a `tier` attribute that distinguishes tier-1 verification (post-backup
//     local check) from tier-2 verification (post-upload remote check), and
//     an `outcome` attribute (`success` / `failure`) so one instrument
//     exposes both flavors.
//   - Base snapshot gauges populated from Kopia, describing the current set
//     of base backups stored on the server. Each recording carries a
//     `tier` attribute (tier-1 for local disk, tier-2 for remote object
//     store) and a `snapshot_source` attribute identifying the Kopia
//     source descriptor (`userName@hostName:path`).
//   - PostgreSQL backup gauges populated from the snapshotted backup
//     metadata, describing the retention window of physical backups per
//     cluster. Each recording carries a `tier` attribute and a
//     `cluster_name` attribute. The `Latest*`/`Oldest*` gauges describe the
//     most recent and oldest backup retained on that tier, and `Backups`
//     counts how many backups are retained.
//
// The snapshot and backup gauges are asynchronous (observable) gauges: a
// collector registers a callback that observes only the series currently
// present.
type ServerBackupMetrics struct {
	Verifications           metric.Int64Counter
	TotalSnapshots          metric.Int64ObservableGauge
	LatestSnapshotSize      metric.Int64ObservableGauge
	LatestSnapshotFileCount metric.Int64ObservableGauge
	LatestSnapshotDirCount  metric.Int64ObservableGauge
	LatestSnapshotTimestamp metric.Int64ObservableGauge
	OldestSnapshotTimestamp metric.Int64ObservableGauge

	Backups               metric.Int64ObservableGauge
	LatestBackupStartTime metric.Int64ObservableGauge
	LatestBackupEndTime   metric.Int64ObservableGauge
	LatestBackupStartLSN  metric.Int64ObservableGauge
	LatestBackupEndLSN    metric.Int64ObservableGauge
	LatestBackupTimeline  metric.Int64ObservableGauge
	OldestBackupStartTime metric.Int64ObservableGauge
	OldestBackupEndTime   metric.Int64ObservableGauge
	OldestBackupStartLSN  metric.Int64ObservableGauge
	OldestBackupEndLSN    metric.Int64ObservableGauge
	OldestBackupTimeline  metric.Int64ObservableGauge
}

// ServerWalMetrics holds OTel instruments for the unified WAL ingest series.
// Every recording carries two attributes: a `tier` discriminator ("1" from
// the WAL server writing to local disk, "2" from the consumer uploading to
// remote storage) and a `cluster_name` identifying the PostgreSQL cluster
// the WAL belongs to.
type ServerWalMetrics struct {
	WalWrittenBytes       metric.Int64Counter
	WalWritten            metric.Int64Counter
	LatestWrittenTime     metric.Int64Gauge
	LatestWrittenLSN      metric.Int64Gauge
	LatestWrittenTimeline metric.Int64Gauge
}

// Centralized metric instrument instances.
//
//nolint:gochecknoglobals
var (
	PluginBackup PluginBackupMetrics
	ServerBackup ServerBackupMetrics
	ServerWal    ServerWalMetrics
)

// All metric instruments are created when this package is loaded, in the
// package that owns the structs. The exported InitXxxMetrics functions are
// retained so tests can rebind the instruments after swapping the meter
// provider.
//
//nolint:gochecknoinits
func init() {
	InitPluginBackupMetrics()
	InitServerBackupMetrics()
	InitServerWalMetrics()
}

// InitPluginBackupMetrics creates OTel instruments for backup lifecycle tracking.
func InitPluginBackupMetrics() {
	meter := otel.Meter(Meter)

	PluginBackup.LatestStartTime, _ = meter.Int64Gauge(PluginBackupLatestStartTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup started."),
		metric.WithUnit("s"),
	)
	PluginBackup.LatestCompletionTime, _ = meter.Int64Gauge(PluginBackupLatestCompletionTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup completed successfully."),
		metric.WithUnit("s"),
	)
	PluginBackup.LatestFailureTime, _ = meter.Int64Gauge(PluginBackupLatestFailureTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup failed."),
		metric.WithUnit("s"),
	)
	PluginBackup.LatestDuration, _ = meter.Float64Gauge(PluginBackupLatestDurationMetric,
		metric.WithDescription("Duration of the most recent backup."),
		metric.WithUnit("s"),
	)
	PluginBackup.Duration, _ = meter.Float64Histogram(PluginBackupDurationMetric,
		metric.WithDescription("Distribution of backup durations, split by the `outcome` "+
			"attribute (`success` / `failure`)."),
		metric.WithUnit("s"),
	)
	PluginBackup.InProgress, _ = meter.Int64UpDownCounter(PluginBackupInProgressMetric,
		metric.WithDescription("Number of backups currently in progress."),
		metric.WithUnit("{backups}"),
	)
	PluginBackup.Runs, _ = meter.Int64Counter(PluginBackupRunsMetric,
		metric.WithDescription("Total number of backup runs, split by the `outcome` "+
			"attribute (`success` / `failure`). Failure data points additionally "+
			"carry a `failure_category` attribute (`"+
			strings.Join(backupfailure.Names(), "`, `")+"`)."),
		metric.WithUnit("{backups}"),
	)
}

// InitServerBackupMetrics creates OTel instruments for server-side backup
// state: verification counters and Kopia base snapshot gauges. It is called
// once automatically when this package is loaded; tests can call it again
// after swapping the meter provider to rebind the instruments.
func InitServerBackupMetrics() {
	meter := otel.Meter(Meter)

	ServerBackup.Verifications, _ = meter.Int64Counter(ServerBackupVerificationsMetric,
		metric.WithDescription("Number of backup verifications, split by the `outcome` "+
			"attribute (`success` / `failure`; `failure` indicates corruption detected). The `tier` "+
			"attribute distinguishes tier-1 (post-backup local check) from tier-2 (post-upload remote check)."),
		metric.WithUnit("{verifications}"),
	)
	ServerBackup.TotalSnapshots, _ = meter.Int64ObservableGauge(ServerBackupSnapshotsMetric,
		metric.WithDescription("Total number of base snapshots, broken down by `tier` and Kopia `snapshot_source`."),
		metric.WithUnit("{snapshots}"),
	)
	ServerBackup.LatestSnapshotSize, _ = meter.Int64ObservableGauge(ServerBackupLatestSnapshotSizeMetric,
		metric.WithDescription("Size of latest base snapshot in bytes (ignoring compression and "+
			"deduplication), broken down by `tier` and Kopia `snapshot_source`."),
		metric.WithUnit("By"),
	)
	ServerBackup.LatestSnapshotFileCount, _ = meter.Int64ObservableGauge(ServerBackupLatestSnapshotFilesMetric,
		metric.WithDescription("Number of files in latest base snapshot, broken down by `tier` and Kopia `snapshot_source`."),
		metric.WithUnit("{files}"),
	)
	ServerBackup.LatestSnapshotDirCount, _ = meter.Int64ObservableGauge(ServerBackupLatestSnapshotDirsMetric,
		metric.WithDescription("Number of directories in latest base snapshot, broken down by `tier` "+
			"and Kopia `snapshot_source`."),
		metric.WithUnit("{directories}"),
	)
	ServerBackup.LatestSnapshotTimestamp, _ = meter.Int64ObservableGauge(ServerBackupLatestSnapshotTimestampMetric,
		metric.WithDescription("Unix epoch timestamp of the latest base snapshot, broken down by "+
			"`tier` and Kopia `snapshot_source`."),
		metric.WithUnit("s"),
	)
	ServerBackup.OldestSnapshotTimestamp, _ = meter.Int64ObservableGauge(ServerBackupOldestSnapshotTimestampMetric,
		metric.WithDescription("Unix epoch timestamp of the oldest base snapshot, broken down by "+
			"`tier` and Kopia `snapshot_source`."),
		metric.WithUnit("s"),
	)

	ServerBackup.Backups, _ = meter.Int64ObservableGauge(ServerBackupBackupsMetric,
		metric.WithDescription("Number of PostgreSQL backups retained, broken down by `tier` and `cluster_name`."),
		metric.WithUnit("{backups}"),
	)
	ServerBackup.LatestBackupStartTime, _ = meter.Int64ObservableGauge(ServerBackupLatestBackupStartTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the latest retained PostgreSQL backup started, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("s"),
	)
	ServerBackup.LatestBackupEndTime, _ = meter.Int64ObservableGauge(ServerBackupLatestBackupEndTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the latest retained PostgreSQL backup completed, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("s"),
	)
	ServerBackup.LatestBackupStartLSN, _ = meter.Int64ObservableGauge(ServerBackupLatestBackupStartLSNMetric,
		metric.WithDescription("Start LSN of the latest retained PostgreSQL backup, in base 10, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("By"),
	)
	ServerBackup.LatestBackupEndLSN, _ = meter.Int64ObservableGauge(ServerBackupLatestBackupEndLSNMetric,
		metric.WithDescription("End LSN of the latest retained PostgreSQL backup, in base 10, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("By"),
	)
	ServerBackup.LatestBackupTimeline, _ = meter.Int64ObservableGauge(ServerBackupLatestBackupTimelineMetric,
		metric.WithDescription("Timeline of the latest retained PostgreSQL backup, "+
			"broken down by `tier` and `cluster_name`."),
	)
	ServerBackup.OldestBackupStartTime, _ = meter.Int64ObservableGauge(ServerBackupOldestBackupStartTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the oldest retained PostgreSQL backup started, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("s"),
	)
	ServerBackup.OldestBackupEndTime, _ = meter.Int64ObservableGauge(ServerBackupOldestBackupEndTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the oldest retained PostgreSQL backup completed, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("s"),
	)
	ServerBackup.OldestBackupStartLSN, _ = meter.Int64ObservableGauge(ServerBackupOldestBackupStartLSNMetric,
		metric.WithDescription("Start LSN of the oldest retained PostgreSQL backup, in base 10, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("By"),
	)
	ServerBackup.OldestBackupEndLSN, _ = meter.Int64ObservableGauge(ServerBackupOldestBackupEndLSNMetric,
		metric.WithDescription("End LSN of the oldest retained PostgreSQL backup, in base 10, "+
			"broken down by `tier` and `cluster_name`."),
		metric.WithUnit("By"),
	)
	ServerBackup.OldestBackupTimeline, _ = meter.Int64ObservableGauge(ServerBackupOldestBackupTimelineMetric,
		metric.WithDescription("Timeline of the oldest retained PostgreSQL backup, "+
			"broken down by `tier` and `cluster_name`."),
	)
}

// InitServerWalMetrics creates OTel instruments for the unified WAL ingest series.
func InitServerWalMetrics() {
	meter := otel.Meter(Meter)

	ServerWal.WalWrittenBytes, _ = meter.Int64Counter(ServerWalWrittenSizeMetric,
		metric.WithDescription("Number of bytes written for WAL files. The `tier` attribute "+
			"distinguishes tier-1 (local disk on the server) from tier-2 (remote storage); "+
			"`cluster_name` identifies the PostgreSQL cluster."),
		metric.WithUnit("By"),
	)
	ServerWal.WalWritten, _ = meter.Int64Counter(ServerWalWrittenMetric,
		metric.WithDescription("Number of WAL files written. The `tier` attribute "+
			"distinguishes tier-1 (local disk on the server) from tier-2 (remote storage); "+
			"`cluster_name` identifies the PostgreSQL cluster."),
		metric.WithUnit("{wals}"),
	)
	ServerWal.LatestWrittenTime, _ = meter.Int64Gauge(ServerWalLatestWrittenTimeMetric,
		metric.WithDescription("Unix epoch timestamp of the most recently written WAL file. The "+
			"`tier` attribute distinguishes tier-1 (local disk) from tier-2 (remote storage); "+
			"`cluster_name` identifies the PostgreSQL cluster."),
		metric.WithUnit("s"),
	)
	ServerWal.LatestWrittenLSN, _ = meter.Int64Gauge(ServerWalLatestWrittenLSNMetric,
		metric.WithDescription("LSN of the most recently written WAL byte, in base 10. The "+
			"`tier` attribute distinguishes tier-1 (flush pointer on local disk) from "+
			"tier-2 (last byte of the most recently archived WAL segment); `cluster_name` "+
			"identifies the PostgreSQL cluster."),
		metric.WithUnit("By"),
	)
	ServerWal.LatestWrittenTimeline, _ = meter.Int64Gauge(ServerWalLatestWrittenTimelineMetric,
		metric.WithDescription("Timeline ID of the most recently completed WAL file. The "+
			"`tier` attribute distinguishes tier-1 (received on the server) from "+
			"tier-2 (archived to remote storage); `cluster_name` identifies the "+
			"PostgreSQL cluster."),
	)
}
