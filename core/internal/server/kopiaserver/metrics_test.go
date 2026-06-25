package kopiaserver

import (
	"context"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// newTestCollector builds a collector wired with a logger and stop channel for
// use in tests that exercise the observable callback directly.
func newTestCollector(tier opentelemetry.Tier) *SnapshotMetricsCollector {
	return &SnapshotMetricsCollector{
		tier:   tier,
		stopCh: make(chan struct{}),
		logger: log.FromContext(context.Background()),
	}
}

// setupTestMeter installs a MeterProvider backed by a ManualReader and rebinds
// the server-backup instruments against it, returning the reader.
func setupTestMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = provider.Shutdown(context.Background())
	})

	opentelemetry.InitServerBackupMetrics()

	return reader
}

// hasBackupSeries reports whether the given int64 gauge has a data point whose
// `cluster_name` attribute equals clusterName.
func hasBackupSeries(rm metricdata.ResourceMetrics, clusterName string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != opentelemetry.ServerBackupBackupsMetric {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				continue
			}
			for _, dp := range g.DataPoints {
				if v, ok := dp.Attributes.Value(attribute.Key(string(opentelemetry.AttributeKeyClusterName))); ok &&
					v.AsString() == clusterName {
					return true
				}
			}
		}
	}

	return false
}

func backupEntry(clusterName string, count int64) backupCacheEntry {
	return backupCacheEntry{
		clusterName: clusterName,
		count:       count,
		latest:      klioclient.BackupMetadata{ClusterName: clusterName, StartedAt: 100, StoppedAt: 200},
		oldest:      klioclient.BackupMetadata{ClusterName: clusterName, StartedAt: 10, StoppedAt: 20},
	}
}

func TestObservableGaugeDropsDisappearedSeries(t *testing.T) {
	reader := setupTestMeter(t)

	c := newTestCollector(opentelemetry.Tier1)
	c.cache = metricsCache{backups: []backupCacheEntry{
		backupEntry("cluster-a", 3),
		backupEntry("cluster-b", 1),
	}}
	c.registerCallback()
	t.Cleanup(c.Stop)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !hasBackupSeries(rm, "cluster-a") {
		t.Fatal("expected cluster-a series on first collection")
	}
	if !hasBackupSeries(rm, "cluster-b") {
		t.Fatal("expected cluster-b series on first collection")
	}

	// Simulate cluster-b's backups being removed.
	c.mu.Lock()
	c.cache = metricsCache{backups: []backupCacheEntry{backupEntry("cluster-a", 3)}}
	c.mu.Unlock()

	rm = metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !hasBackupSeries(rm, "cluster-a") {
		t.Fatal("expected cluster-a series to remain after cluster-b removed")
	}
	if hasBackupSeries(rm, "cluster-b") {
		t.Fatal("expected cluster-b series to disappear after its backups were removed")
	}
}

func TestTwoTiersObserveSameInstrumentDistinctSeries(t *testing.T) {
	reader := setupTestMeter(t)

	for _, tier := range []opentelemetry.Tier{opentelemetry.Tier1, opentelemetry.Tier2} {
		c := newTestCollector(tier)
		c.cache = metricsCache{backups: []backupCacheEntry{backupEntry("cluster-a", 1)}}
		c.registerCallback()
		t.Cleanup(c.Stop)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	tiers := make(map[string]struct{})
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != opentelemetry.ServerBackupBackupsMetric {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				continue
			}
			for _, dp := range g.DataPoints {
				if v, ok := dp.Attributes.Value(attribute.Key(string(opentelemetry.AttributeKeyTier))); ok {
					tiers[v.AsString()] = struct{}{}
				}
			}
		}
	}

	if len(tiers) != 2 {
		t.Fatalf("expected 2 distinct tier series for cluster-a, got %d: %v", len(tiers), tiers)
	}
}

func TestSnapshotStatsIncrementFirstSnapshotShouldInitializeAndUpdate(t *testing.T) {
	s := snapshotStats{}
	ds := kopia.DirectorySummary{
		TotalFileSize:  123,
		TotalFileCount: 5,
		TotalDirCount:  2,
	}

	s.update(42, &ds)

	if s.snapshotCount != 1 {
		t.Fatalf("expected snapshotCount=1, got %d (deferred update on value receiver is lost)", s.snapshotCount)
	}
	if s.oldestSnapshotTimestamp != 42 || s.latestSnapshotTimestamp != 42 {
		t.Fatalf("expected timestamps oldest=42 and latest=42, got oldest=%v latest=%v",
			s.oldestSnapshotTimestamp, s.latestSnapshotTimestamp)
	}
	if s.snapshotSize != 123 || s.latestSnapshotFilesCount != 5 || s.latestSnapshotDirCount != 2 {
		t.Fatalf("expected latest snapshot details from ds, got size=%d files=%d dirs=%d",
			s.snapshotSize, s.latestSnapshotFilesCount, s.latestSnapshotDirCount)
	}
}

func TestSnapshotStatsTracksLatestAndOldestTimestamps(t *testing.T) {
	s := snapshotStats{}
	first := kopia.DirectorySummary{TotalFileSize: 100, TotalFileCount: 1, TotalDirCount: 1}
	latest := kopia.DirectorySummary{TotalFileSize: 300, TotalFileCount: 3, TotalDirCount: 3}
	oldest := kopia.DirectorySummary{TotalFileSize: 50, TotalFileCount: 9, TotalDirCount: 9}

	// Feed timestamps out of order: middle, newest, oldest.
	s.update(200, &first)
	s.update(500, &latest)
	s.update(10, &oldest)

	if s.snapshotCount != 3 {
		t.Fatalf("expected snapshotCount=3, got %d", s.snapshotCount)
	}
	if s.oldestSnapshotTimestamp != 10 {
		t.Fatalf("expected oldest timestamp=10, got %d", s.oldestSnapshotTimestamp)
	}
	if s.latestSnapshotTimestamp != 500 {
		t.Fatalf("expected latest timestamp=500, got %d", s.latestSnapshotTimestamp)
	}
	// Size/files/dirs must reflect the latest (max timestamp) snapshot.
	if s.snapshotSize != 300 || s.latestSnapshotFilesCount != 3 || s.latestSnapshotDirCount != 3 {
		t.Fatalf("expected latest snapshot details from the newest snapshot, got size=%d files=%d dirs=%d",
			s.snapshotSize, s.latestSnapshotFilesCount, s.latestSnapshotDirCount)
	}
}

func TestBuildBackupGroupsSelectsLatestOldestAndCount(t *testing.T) {
	clusterA := kopia.SourceInfo{Host: "node-a", UserName: "postgres", Path: "/data_meta"}
	clusterB := kopia.SourceInfo{Host: "node-b", UserName: "postgres", Path: "/data_meta"}

	snapshots := []kopia.Manifest{
		{ID: "a-mid", Source: clusterA, EndTime: 200},
		{ID: "a-new", Source: clusterA, EndTime: 500},
		{ID: "a-old", Source: clusterA, EndTime: 10},
		{ID: "b-only", Source: clusterB, EndTime: 42},
	}

	groups := buildBackupGroups(snapshots)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	a := groups[clusterA]
	if a == nil {
		t.Fatal("expected a group for clusterA")
	}
	if a.count != 3 {
		t.Fatalf("expected clusterA count=3, got %d", a.count)
	}
	if a.latestID != "a-new" {
		t.Fatalf("expected clusterA latestID=a-new, got %s", a.latestID)
	}
	if a.oldestID != "a-old" {
		t.Fatalf("expected clusterA oldestID=a-old, got %s", a.oldestID)
	}

	b := groups[clusterB]
	if b == nil {
		t.Fatal("expected a group for clusterB")
	}
	if b.count != 1 {
		t.Fatalf("expected clusterB count=1, got %d", b.count)
	}
	// With a single backup, latest and oldest coincide.
	if b.latestID != "b-only" || b.oldestID != "b-only" {
		t.Fatalf("expected clusterB latest==oldest==b-only, got latest=%s oldest=%s", b.latestID, b.oldestID)
	}
}
