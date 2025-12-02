package kopiaserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

const (
	kopiaServerUptimeMetricName        = "klio.base.uptime"
	totalSnapshotsMetricName           = "klio.base.snapshots"
	latestSnapshotSizeBytesMetricName  = "klio.base.latest_snapshot_size"
	latestSnapshotFileCountMetricName  = "klio.base.latest_snapshot_files"
	latestSnapshotDirCountMetricName   = "klio.base.latest_snapshot_dirs"
	latestSnapshotAgeSecondsMetricName = "klio.base.latest_snapshot_age"
	oldestSnapshotAgeMetricName        = "klio.base.oldest_snapshot_age"
)

// SnapshotMetrics holds OpenTelemetry metrics for Kopia snapshots.
type SnapshotMetrics struct {
	totalSnapshots          metric.Int64Gauge
	latestSnapshotSize      metric.Int64Gauge
	latestSnapshotFileCount metric.Int64Gauge
	latestSnapshotDirCount  metric.Int64Gauge
	latestSnapshotAge       metric.Float64Gauge
	oldestSnapshotAge       metric.Float64Gauge
}

// newSnapshotMetrics creates new OpenTelemetry metrics for Kopia snapshots.
func newSnapshotMetrics() (*SnapshotMetrics, error) {
	meter := otel.Meter(opentelemetry.Meter)

	totalSnapshots, err := meter.Int64Gauge(
		totalSnapshotsMetricName,
		metric.WithDescription("Total number of base snapshots"),
		metric.WithUnit("{snapshots}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create total snapshots metric: %w", err)
	}

	latestSnapshotSize, err := meter.Int64Gauge(
		latestSnapshotSizeBytesMetricName,
		metric.WithDescription("Size of latest base snapshot in bytes (ignoring compression and deduplication)"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot size metric: %w", err)
	}

	latestSnapshotFileCount, err := meter.Int64Gauge(
		latestSnapshotFileCountMetricName,
		metric.WithDescription("Number of files in latest base snapshot"),
		metric.WithUnit("{files}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot file count metric: %w", err)
	}

	latestSnapshotDirCount, err := meter.Int64Gauge(
		latestSnapshotDirCountMetricName,
		metric.WithDescription("Number of directories in latest base snapshots"),
		metric.WithUnit("{directories}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot dir count metric: %w", err)
	}

	latestSnapshotAge, err := meter.Float64Gauge(
		latestSnapshotAgeSecondsMetricName,
		metric.WithDescription("Age of latest base snapshot in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot age metric: %w", err)
	}

	oldestSnapshotAge, err := meter.Float64Gauge(
		oldestSnapshotAgeMetricName,
		metric.WithDescription("Age of oldest base snapshot in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot age metric: %w", err)
	}

	return &SnapshotMetrics{
		totalSnapshots:          totalSnapshots,
		latestSnapshotSize:      latestSnapshotSize,
		latestSnapshotFileCount: latestSnapshotFileCount,
		latestSnapshotDirCount:  latestSnapshotDirCount,
		latestSnapshotAge:       latestSnapshotAge,
		oldestSnapshotAge:       oldestSnapshotAge,
	}, nil
}

// SnapshotMetricsCollector periodically collects Kopia snapshot metrics.
type SnapshotMetricsCollector struct {
	metrics    *SnapshotMetrics
	configPath string
	password   string
	interval   time.Duration
	logger     log.Logger
	stopCh     chan struct{}
}

// newSnapshotMetricsCollector creates a new snapshot metrics collector.
func newSnapshotMetricsCollector(
	configPath, password string, interval time.Duration, logger log.Logger,
) (*SnapshotMetricsCollector, error) {
	metrics, err := newSnapshotMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot metrics: %w", err)
	}

	return &SnapshotMetricsCollector{
		metrics:    metrics,
		configPath: configPath,
		password:   password,
		interval:   interval,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}, nil
}

// Start begins the periodic collection of snapshot metrics.
func (c *SnapshotMetricsCollector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.logger.Info("Starting Kopia snapshot metrics collection", "interval", c.interval)
	// Collect metrics immediately on start
	c.collectMetrics(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.collectMetrics(ctx)
		}
	}
}

// Stop stops the metrics collection.
func (c *SnapshotMetricsCollector) Stop() {
	close(c.stopCh)
}

// collectMetrics executes kopia snapshot list and extracts metrics.
func (c *SnapshotMetricsCollector) collectMetrics(ctx context.Context) {
	c.logger.Debug("Collecting Kopia snapshot metrics")
	snapshots, err := c.getSnapshots(ctx)
	if err != nil {
		c.logger.Warning("Failed to get Kopia snapshots", "error", err)
		return
	}

	c.updateMetrics(ctx, snapshots)
}

// getSnapshots executes kopia snapshot list --all --json and parses the output.
func (c *SnapshotMetricsCollector) getSnapshots(ctx context.Context) ([]snapshot.Manifest, error) {
	contextLogger := log.FromContext(ctx)

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return nil, fmt.Errorf("kopia binary not found: %w", err)
	}

	args := []string{
		"snapshot",
		"list",
		"--all",
		"--json",
		"--config-file=" + c.configPath,
		"--disable-file-logging",
		"--json-log-console",
	}

	contextLogger.Info("Collecting Kopia snapshot information", "args", args)
	cmd := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	cmd.Env = []string{
		"KOPIA_PASSWORD=" + c.password,
	}

	var outputBuffer, errorBuffer bytes.Buffer
	cmd.Stdout = &outputBuffer
	cmd.Stderr = &errorBuffer

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to execute kopia snapshot list: %w [stdout\n%s\n\nstderr\n%s]",
			err,
			outputBuffer.String(),
			errorBuffer.String(),
		)
	}

	var snapshots []snapshot.Manifest
	if err := json.Unmarshal(outputBuffer.Bytes(), &snapshots); err != nil {
		return nil, fmt.Errorf("failed to parse kopia snapshot JSON: %w", err)
	}

	return snapshots, nil
}

type snapshotStats struct {
	oldestSnapshotAge        float64
	latestSnapshotAge        float64
	snapshotCount            int64
	snapshotSize             int64
	latestSnapshotFilesCount int64
	latestSnapshotDirCount   int64
}

func (s *snapshotStats) update(age float64, ds *fs.DirectorySummary) {
	defer func() {
		s.snapshotCount++
	}()

	if s.snapshotCount == 0 {
		s.oldestSnapshotAge = age
		s.latestSnapshotAge = age
		s.snapshotSize = ds.TotalFileSize
		s.latestSnapshotFilesCount = ds.TotalFileCount
		s.latestSnapshotDirCount = ds.TotalDirCount

		return
	}

	if age > s.oldestSnapshotAge {
		s.oldestSnapshotAge = age
	}
	if age < s.latestSnapshotAge {
		s.latestSnapshotAge = age
		s.snapshotSize = ds.TotalFileSize
		s.latestSnapshotFilesCount = ds.TotalFileCount
		s.latestSnapshotDirCount = ds.TotalDirCount
	}
}

// updateMetrics updates OpenTelemetry metrics based on snapshot data.
func (c *SnapshotMetricsCollector) updateMetrics(ctx context.Context, snapshots []snapshot.Manifest) {
	now := time.Now()

	stats := make(map[snapshot.SourceInfo]snapshotStats)
	for _, s := range snapshots {
		snapshotLogger := log.FromContext(ctx).WithValues("snapshotID", s.ID, "source", s.Source)
		snapshotLogger.Debug("Processing snapshot")

		age := now.Sub(s.EndTime.ToTime()).Seconds()
		ds := s.RootEntry.DirSummary

		v := stats[s.Source]
		v.update(age, ds)
		stats[s.Source] = v
	}

	for origin, stat := range stats {
		attrs := []attribute.KeyValue{
			attribute.String("snapshot_source", origin.String()),
		}
		c.metrics.totalSnapshots.Record(ctx, stat.snapshotCount, metric.WithAttributes(attrs...))
		c.metrics.oldestSnapshotAge.Record(ctx, stat.oldestSnapshotAge, metric.WithAttributes(attrs...))
		c.metrics.latestSnapshotAge.Record(ctx, stat.latestSnapshotAge, metric.WithAttributes(attrs...))
		c.metrics.latestSnapshotSize.Record(ctx, stat.snapshotSize, metric.WithAttributes(attrs...))
		c.metrics.latestSnapshotDirCount.Record(ctx, stat.latestSnapshotDirCount, metric.WithAttributes(attrs...))
		c.metrics.latestSnapshotFileCount.Record(ctx, stat.latestSnapshotFilesCount, metric.WithAttributes(attrs...))
	}
}
