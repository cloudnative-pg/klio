/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package opentelemetry

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
)

// RecordDuration records d (in nanoseconds) on the given duration histogram,
// tagging it with the outcome derived from err alongside any extra attributes
// (e.g. tier or stage). It is a no-op when h is nil, so call sites recording on
// the package-level WAL duration instruments need not guard against an
// uninitialized instrument themselves.
func RecordDuration(
	ctx context.Context,
	h metric.Int64Histogram,
	d time.Duration,
	err error,
	attrs ...attribute.KeyValue,
) {
	if h == nil {
		return
	}

	outcome := OutcomeSuccess
	if err != nil {
		outcome = OutcomeFailure
	}

	attrs = append(attrs, outcome.Attribute())
	h.Record(ctx, d.Nanoseconds(), metric.WithAttributes(attrs...))
}

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
	// Tier2UploadSpan is the span name for archiving a single WAL file to
	// tier-2 (remote storage) on the server-side consumer.
	Tier2UploadSpan = "tier2_upload"
	// GetWalSpan is the span name for getting WAL files.
	GetWalSpan = "get_wal"
	// BackupSpan is the span name for the backup entry point.
	BackupSpan = "backup"
	// BackupRunSpan is the span name for running a backup.
	BackupRunSpan = "backup_run"
	// BackupVerifySpan is the span name for verifying a backup.
	BackupVerifySpan = "backup_verify"
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

	ServerWalBlockDurationMetric  = "klio.server.wal.block_duration"
	ServerWalGetDurationMetric    = "klio.server.wal.get_duration"
	ServerWalUploadDurationMetric = "klio.server.wal.upload_duration"

	ClientWalBlockDurationMetric = "klio.client.wal.block_duration"
	ClientWalTimelineMetric      = "klio.client.wal.timeline"

	ServerUptimeMetric                        = "klio.server.uptime"
	ServerBackupSnapshotsMetric               = "klio.server.backup.snapshots"
	ServerBackupLatestSnapshotSizeMetric      = "klio.server.backup.latest_snapshot_size"
	ServerBackupLatestSnapshotFilesMetric     = "klio.server.backup.latest_snapshot_files"
	ServerBackupLatestSnapshotDirsMetric      = "klio.server.backup.latest_snapshot_dirs"
	ServerBackupLatestSnapshotTimestampMetric = "klio.server.backup.latest_snapshot_timestamp"
	ServerBackupOldestSnapshotTimestampMetric = "klio.server.backup.oldest_snapshot_timestamp"
	ServerBackupVerificationsMetric           = "klio.server.backup.verifications"

	ServerBackupRelayMetric       = "klio.server.backup.relay"
	ServerBackupMaintenanceMetric = "klio.server.backup.maintenance"

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
// Every instrument carries a `cluster_name` attribute identifying the
// PostgreSQL cluster the plugin sidecar serves, so panels can attribute backup
// activity per cluster even when several clusters share a namespace. The Runs
// counter additionally carries an `outcome` attribute (`success` / `failure`)
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
//     source descriptor, formatted `userName@hostName:path`. Its `hostName`
//     component is the PostgreSQL cluster name, which the Grafana dashboard
//     relies on to attribute snapshots per cluster.
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
	Relay                   metric.Int64Counter
	Maintenance             metric.Int64Counter
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
	BlockDuration         metric.Int64Histogram
	GetDuration           metric.Int64Histogram
	UploadDuration        metric.Int64Histogram
}

// ClientWalMetrics holds OTel instruments for the client-side WAL streaming
// path. BlockDuration is recorded once per WAL block and carries a `stage`
// attribute (receive, wrap, send) alongside `outcome`. Timeline holds
// the timeline the client is currently streaming, updated on each switch.
type ClientWalMetrics struct {
	BlockDuration metric.Int64Histogram
	Timeline      metric.Int64Gauge
}

// Centralized metric instrument instances.
//
//nolint:gochecknoglobals
var (
	PluginBackup PluginBackupMetrics
	ServerBackup ServerBackupMetrics
	ServerWal    ServerWalMetrics
	ClientWal    ClientWalMetrics
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
	InitClientWalMetrics()
}

// InitPluginBackupMetrics creates OTel instruments for backup lifecycle tracking.
func InitPluginBackupMetrics() {
	meter := otel.Meter(Meter)

	PluginBackup.LatestStartTime, _ = meter.Int64Gauge(PluginBackupLatestStartTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup started, "+
			"broken down by `cluster_name`."),
		metric.WithUnit("s"),
	)
	PluginBackup.LatestCompletionTime, _ = meter.Int64Gauge(PluginBackupLatestCompletionTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup completed "+
			"successfully, broken down by `cluster_name`."),
		metric.WithUnit("s"),
	)
	PluginBackup.LatestFailureTime, _ = meter.Int64Gauge(PluginBackupLatestFailureTimeMetric,
		metric.WithDescription("Unix epoch timestamp when the most recent backup failed, "+
			"broken down by `cluster_name`."),
		metric.WithUnit("s"),
	)
	PluginBackup.LatestDuration, _ = meter.Float64Gauge(PluginBackupLatestDurationMetric,
		metric.WithDescription("Duration of the most recent backup, broken down by `cluster_name`."),
		metric.WithUnit("s"),
	)
	PluginBackup.Duration, _ = meter.Float64Histogram(PluginBackupDurationMetric,
		metric.WithDescription("Distribution of backup durations, broken down by `cluster_name` "+
			"and split by the `outcome` attribute (`success` / `failure`)."),
		metric.WithUnit("s"),
	)
	PluginBackup.InProgress, _ = meter.Int64UpDownCounter(PluginBackupInProgressMetric,
		metric.WithDescription("Number of backups currently in progress, broken down by `cluster_name`."),
		metric.WithUnit("{backups}"),
	)
	PluginBackup.Runs, _ = meter.Int64Counter(PluginBackupRunsMetric,
		metric.WithDescription("Total number of backup runs, broken down by `cluster_name` and "+
			"split by the `outcome` attribute (`success` / `failure`). Failure data points "+
			"additionally carry a `failure_category` attribute (`"+
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
	ServerBackup.Relay, _ = meter.Int64Counter(ServerBackupRelayMetric,
		metric.WithDescription("Number of tier2 relay attempts after a backup (migrating the backup to "+
			"tier2 and verifying it there), split by the `cluster_name` and `outcome` "+
			"(`success` / `failure`) attributes. A failure means the backup did not reach tier2 on that "+
			"attempt; the task is retried."),
		metric.WithUnit("{relays}"),
	)
	ServerBackup.Maintenance, _ = meter.Int64Counter(ServerBackupMaintenanceMetric,
		metric.WithDescription("Number of maintenance runs after a backup (base-snapshot retention and "+
			"WAL cleanup), split by the `cluster_name`, `tier` (`tier1` / `tier2`) and `outcome` "+
			"(`success` / `failure`) attributes."),
		metric.WithUnit("{runs}"),
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
	ServerWal.BlockDuration, _ = meter.Int64Histogram(ServerWalBlockDurationMetric,
		metric.WithDescription("Distribution of per-block WAL processing durations on the server. "+
			"The `path` attribute is `put` (ingest) or `get` (serve); the `stage` attribute splits "+
			"each path (put: `wrap`, `write`, `flush`; get: `read`, `unwrap`, `send`); the `tier` "+
			"attribute is the serving tier (put is always tier-1; get is tier-1 for local serves or "+
			"tier-2 for serves from remote storage); `cluster_name` identifies the PostgreSQL cluster; "+
			"`outcome` is `success` or `failure`."),
		metric.WithUnit("ns"),
	)
	ServerWal.GetDuration, _ = meter.Int64Histogram(ServerWalGetDurationMetric,
		metric.WithDescription("Distribution of per-file WAL get durations on the server (gRPC "+
			"retrieval of a complete WAL file). The `tier` attribute is the tier that served the "+
			"request (tier-1 from local disk or tier-2 from remote storage); `cluster_name` identifies "+
			"the PostgreSQL cluster; `outcome` is `success` or `failure`."),
		metric.WithUnit("ns"),
	)
	ServerWal.UploadDuration, _ = meter.Int64Histogram(ServerWalUploadDurationMetric,
		metric.WithDescription("Distribution of per-file WAL upload durations on the server (tier-2 "+
			"archival leg to remote storage). Always tagged tier-2; `cluster_name` identifies the "+
			"PostgreSQL cluster; `outcome` is `success` or `failure`."),
		metric.WithUnit("ns"),
	)
}

// InitClientWalMetrics creates OTel instruments for the client-side WAL
// streaming path. It is called once automatically when this package is loaded;
// tests can call it again after swapping the meter provider to rebind the
// instruments.
func InitClientWalMetrics() {
	meter := otel.Meter(Meter)

	ClientWal.BlockDuration, _ = meter.Int64Histogram(ClientWalBlockDurationMetric,
		metric.WithDescription("Distribution of per-block WAL durations on the client. Carries "+
			"`path=put` and the `send` stage (gRPC send of a WAL block to the Klio server); "+
			"`outcome` is `success` or `failure`."),
		metric.WithUnit("ns"),
	)
	ClientWal.Timeline, _ = meter.Int64Gauge(ClientWalTimelineMetric,
		metric.WithDescription("Timeline ID the WAL streaming client is currently streaming."),
	)
}
