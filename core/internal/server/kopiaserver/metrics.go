package kopiaserver

import (
	"context"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// SnapshotMetricsCollector periodically collects Kopia snapshot metrics.
type SnapshotMetricsCollector struct {
	interval time.Duration
	logger   log.Logger
	stopCh   chan struct{}
	kopia    *kopia.Client
}

// newSnapshotMetricsCollector creates a new snapshot metrics collector.
func newSnapshotMetricsCollector(
	configPath string, interval time.Duration, logger log.Logger,
) (*SnapshotMetricsCollector, error) {
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return nil, err
	}

	return &SnapshotMetricsCollector{
		interval: interval,
		logger:   logger,
		stopCh:   make(chan struct{}),
		kopia: &kopia.Client{
			ConfigFile:  configPath,
			KopiaBinary: kopiaBinary,
		},
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
	snapshots, err := c.kopia.ListSnapshots(ctx, nil, c.logger.Debug)
	if err != nil {
		c.logger.Warning("Failed to get Kopia snapshots", "error", err)
		return
	}

	c.updateMetrics(ctx, snapshots)
}

type snapshotStats struct {
	oldestSnapshotAge        float64
	latestSnapshotAge        float64
	snapshotCount            int64
	snapshotSize             int64
	latestSnapshotFilesCount int64
	latestSnapshotDirCount   int64
}

func (s *snapshotStats) update(age float64, ds *kopia.DirectorySummary) {
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
func (c *SnapshotMetricsCollector) updateMetrics(ctx context.Context, snapshots []kopia.Manifest) {
	now := time.Now()

	stats := make(map[kopia.SourceInfo]snapshotStats)
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
			opentelemetry.AttributeKeySnapshotSource.Of(origin.String()),
		}
		opentelemetry.ServerBackup.TotalSnapshots.Record(ctx, stat.snapshotCount, metric.WithAttributes(attrs...))
		opentelemetry.ServerBackup.OldestSnapshotAge.Record(ctx, stat.oldestSnapshotAge, metric.WithAttributes(attrs...))
		opentelemetry.ServerBackup.LatestSnapshotAge.Record(ctx, stat.latestSnapshotAge, metric.WithAttributes(attrs...))
		opentelemetry.ServerBackup.LatestSnapshotSize.Record(ctx, stat.snapshotSize, metric.WithAttributes(attrs...))
		opentelemetry.ServerBackup.LatestSnapshotDirCount.Record(
			ctx, stat.latestSnapshotDirCount, metric.WithAttributes(attrs...))
		opentelemetry.ServerBackup.LatestSnapshotFileCount.Record(
			ctx, stat.latestSnapshotFilesCount, metric.WithAttributes(attrs...))
	}
}
