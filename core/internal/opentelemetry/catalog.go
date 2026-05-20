package opentelemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Tracer name constants.
const (
	// TracerBackup is the tracer name for backup operations.
	TracerBackup = "klio.backup"
	// TracerConsumer is the tracer name for the WAL consumer.
	TracerConsumer = "klio.consumer"
	// TracerWalServer is the tracer name for the WAL server.
	TracerWalServer = "klio.wal_server"
	// TracerWalClient is the tracer name for the WAL client.
	TracerWalClient = "klio.wal_client"
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
	BackupLatestStartTimeMetric      = "klio.backup.latest_start_time"
	BackupLatestCompletionTimeMetric = "klio.backup.latest_completion_time"
	BackupLatestFailureTimeMetric    = "klio.backup.latest_failure_time"
	BackupLatestDurationMetric       = "klio.backup.latest_duration_seconds"
	BackupRunningMetric              = "klio.backup.running"
	BackupFailuresMetric             = "klio.backup.failures"
	BackupSuccessesMetric            = "klio.backup.successes"
	BackupVerificationsMetric        = "klio.backup.verifications"
	BackupVerificationFailuresMetric = "klio.backup.verification_failures"

	ConsumerVerificationSuccessMetric   = "klio.consumer.backup_verification_success"
	ConsumerVerificationFailureMetric   = "klio.consumer.backup_verification_failure"
	ConsumerWrittenSizeMetric           = "klio.consumer.written_size"
	ConsumerWrittenMetric               = "klio.consumer.written"
	ConsumerLatestWrittenTimeMetric     = "klio.consumer.latest_written_time"
	ConsumerLatestWrittenLSNMetric      = "klio.consumer.latest_written_lsn"
	ConsumerLatestWrittenTimelineMetric = "klio.consumer.latest_written_timeline"

	WalServerWrittenSizeMetric           = "klio.wal.written_size"
	WalServerWrittenMetric               = "klio.wal.written"
	WalServerLatestWrittenTimeMetric     = "klio.wal.latest_written_time"
	WalServerLatestWrittenLSNMetric      = "klio.wal.latest_written_lsn"
	WalServerLatestWrittenTimelineMetric = "klio.wal.latest_written_timeline"

	KopiaServerUptimeMetric       = "klio.base.uptime"
	SnapshotTotalMetric           = "klio.base.snapshots"
	SnapshotLatestSizeMetric      = "klio.base.latest_snapshot_size"
	SnapshotLatestFileCountMetric = "klio.base.latest_snapshot_files"
	SnapshotLatestDirCountMetric  = "klio.base.latest_snapshot_dirs"
	SnapshotLatestAgeMetric       = "klio.base.latest_snapshot_age"
	SnapshotOldestAgeMetric       = "klio.base.oldest_snapshot_age"

	QueueMessagesMetric = "klio.queue.messages"
	QueueBytesMetric    = "klio.queue.bytes"
)

// BackupMetrics holds OTel instruments for backup lifecycle tracking.
type BackupMetrics struct {
	LatestStartTime      metric.Float64Gauge
	LatestCompletionTime metric.Float64Gauge
	LatestFailureTime    metric.Float64Gauge
	LatestDuration       metric.Float64Gauge
	Running              metric.Int64Gauge
	Failures             metric.Int64Counter
	Successes            metric.Int64Counter
	Verifications        metric.Int64Counter
	VerificationFailures metric.Int64Counter
}

// ConsumerMetrics holds OTel instruments for the WAL consumer.
type ConsumerMetrics struct {
	VerificationSuccess   metric.Int64Counter
	VerificationFailure   metric.Int64Counter
	WalWrittenBytes       metric.Int64Counter
	WalWritten            metric.Int64Counter
	LatestWrittenTime     metric.Float64Gauge
	LatestWrittenLSN      metric.Int64Gauge
	LatestWrittenTimeline metric.Int64Gauge
}

// WalServerMetrics holds OTel instruments for the WAL server.
type WalServerMetrics struct {
	WalWrittenBytes       metric.Int64Counter
	WalWritten            metric.Int64Counter
	LatestWrittenTime     metric.Float64Gauge
	LatestWrittenLSN      metric.Int64Gauge
	LatestWrittenTimeline metric.Int64Gauge
}

// SnapshotMetrics holds OTel instruments for Kopia snapshot gauges.
type SnapshotMetrics struct {
	TotalSnapshots          metric.Int64Gauge
	LatestSnapshotSize      metric.Int64Gauge
	LatestSnapshotFileCount metric.Int64Gauge
	LatestSnapshotDirCount  metric.Int64Gauge
	LatestSnapshotAge       metric.Float64Gauge
	OldestSnapshotAge       metric.Float64Gauge
}

// Centralized metric instrument instances.
//
//nolint:gochecknoglobals
var (
	Backup    BackupMetrics
	Consumer  ConsumerMetrics
	WalServer WalServerMetrics
	Snapshot  SnapshotMetrics
)

// InitBackupMetrics creates OTel instruments for backup lifecycle tracking.
func InitBackupMetrics() {
	meter := otel.Meter(Meter)

	Backup.LatestStartTime, _ = meter.Float64Gauge(BackupLatestStartTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup started."),
		metric.WithUnit("s"),
	)
	Backup.LatestCompletionTime, _ = meter.Float64Gauge(BackupLatestCompletionTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup completed successfully."),
		metric.WithUnit("s"),
	)
	Backup.LatestFailureTime, _ = meter.Float64Gauge(BackupLatestFailureTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup failed."),
		metric.WithUnit("s"),
	)
	Backup.LatestDuration, _ = meter.Float64Gauge(BackupLatestDurationMetric,
		metric.WithDescription("Duration of the most recent backup in seconds."),
		metric.WithUnit("s"),
	)
	Backup.Running, _ = meter.Int64Gauge(BackupRunningMetric,
		metric.WithDescription("Whether a backup is currently running (1) or not (0)."),
	)
	Backup.Failures, _ = meter.Int64Counter(BackupFailuresMetric,
		metric.WithDescription("Total number of failed backups."),
	)
	Backup.Successes, _ = meter.Int64Counter(BackupSuccessesMetric,
		metric.WithDescription("Total number of successful backups."),
	)
	Backup.Verifications, _ = meter.Int64Counter(BackupVerificationsMetric,
		metric.WithDescription("Total number of backup verification attempts."),
	)
	Backup.VerificationFailures, _ = meter.Int64Counter(BackupVerificationFailuresMetric,
		metric.WithDescription("Total number of backup verification failures."),
	)
}

// InitConsumerMetrics creates OTel instruments for the WAL consumer.
func InitConsumerMetrics() {
	meter := otel.Meter(Meter)

	Consumer.VerificationSuccess, _ = meter.Int64Counter(ConsumerVerificationSuccessMetric,
		metric.WithDescription("Number of successful backup verifications."),
		metric.WithUnit("{verifications}"),
	)
	Consumer.VerificationFailure, _ = meter.Int64Counter(ConsumerVerificationFailureMetric,
		metric.WithDescription("Number of failed backup verifications (corruption detected)."),
		metric.WithUnit("{verifications}"),
	)
	Consumer.WalWrittenBytes, _ = meter.Int64Counter(ConsumerWrittenSizeMetric,
		metric.WithDescription("Number of bytes written to Tier 2 for the WAL files."),
		metric.WithUnit("By"),
	)
	Consumer.WalWritten, _ = meter.Int64Counter(ConsumerWrittenMetric,
		metric.WithDescription("Number of WAL files written."),
		metric.WithUnit("{wals}"),
	)
	Consumer.LatestWrittenTime, _ = meter.Float64Gauge(ConsumerLatestWrittenTimeMetric,
		metric.WithDescription("Unix epoch timestamp of the most recently written WAL file to Tier 2."),
		metric.WithUnit("s"),
	)
	Consumer.LatestWrittenLSN, _ = meter.Int64Gauge(ConsumerLatestWrittenLSNMetric,
		metric.WithDescription(
			"The LSN of last byte of the WAL file most recently archived on tier 2 in base 10."),
		metric.WithUnit("By"),
	)
	Consumer.LatestWrittenTimeline, _ = meter.Int64Gauge(ConsumerLatestWrittenTimelineMetric,
		metric.WithDescription(
			"Timeline ID of the most recently archived WAL file on Tier 2."),
	)
}

// InitWalServerMetrics creates OTel instruments for the WAL server.
func InitWalServerMetrics() {
	meter := otel.Meter(Meter)

	WalServer.WalWrittenBytes, _ = meter.Int64Counter(WalServerWrittenSizeMetric,
		metric.WithDescription("Number of bytes written to disk for the WAL files."),
		metric.WithUnit("By"),
	)
	WalServer.WalWritten, _ = meter.Int64Counter(WalServerWrittenMetric,
		metric.WithDescription("Number of WAL files written."),
		metric.WithUnit("{wals}"),
	)
	WalServer.LatestWrittenTime, _ = meter.Float64Gauge(WalServerLatestWrittenTimeMetric,
		metric.WithDescription("Unix epoch timestamp of the most recently written WAL file to disk."),
		metric.WithUnit("s"),
	)
	WalServer.LatestWrittenLSN, _ = meter.Int64Gauge(WalServerLatestWrittenLSNMetric,
		metric.WithDescription(
			"The LSN of the most recently flushed WAL byte on Tier 1 in base 10."),
		metric.WithUnit("By"),
	)
	WalServer.LatestWrittenTimeline, _ = meter.Int64Gauge(WalServerLatestWrittenTimelineMetric,
		metric.WithDescription(
			"Timeline ID of the most recently completed WAL file received on Tier 1."),
	)
}

// InitSnapshotMetrics creates OTel instruments for Kopia snapshot gauges.
func InitSnapshotMetrics() {
	meter := otel.Meter(Meter)

	Snapshot.TotalSnapshots, _ = meter.Int64Gauge(SnapshotTotalMetric,
		metric.WithDescription("Total number of base snapshots."),
		metric.WithUnit("{snapshots}"),
	)
	Snapshot.LatestSnapshotSize, _ = meter.Int64Gauge(SnapshotLatestSizeMetric,
		metric.WithDescription("Size of latest base snapshot in bytes (ignoring compression and deduplication)."),
		metric.WithUnit("By"),
	)
	Snapshot.LatestSnapshotFileCount, _ = meter.Int64Gauge(SnapshotLatestFileCountMetric,
		metric.WithDescription("Number of files in latest base snapshot."),
		metric.WithUnit("{files}"),
	)
	Snapshot.LatestSnapshotDirCount, _ = meter.Int64Gauge(SnapshotLatestDirCountMetric,
		metric.WithDescription("Number of directories in latest base snapshot."),
		metric.WithUnit("{directories}"),
	)
	Snapshot.LatestSnapshotAge, _ = meter.Float64Gauge(SnapshotLatestAgeMetric,
		metric.WithDescription("Age of latest base snapshot in seconds."),
		metric.WithUnit("s"),
	)
	Snapshot.OldestSnapshotAge, _ = meter.Float64Gauge(SnapshotOldestAgeMetric,
		metric.WithDescription("Age of oldest base snapshot in seconds."),
		metric.WithUnit("s"),
	)
}
