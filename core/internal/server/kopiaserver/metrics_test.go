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

package kopiaserver

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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

func TestBuildBackupGroupsSortsBySourceAndEndTime(t *testing.T) {
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

	ids := func(group []kopia.Manifest) []string {
		result := make([]string, 0, len(group))
		for _, s := range group {
			result = append(result, s.ID)
		}

		return result
	}

	if got := ids(groups[clusterA]); !slices.Equal(got, []string{"a-old", "a-mid", "a-new"}) {
		t.Fatalf("expected clusterA sorted oldest to newest, got %v", got)
	}
	if got := ids(groups[clusterB]); !slices.Equal(got, []string{"b-only"}) {
		t.Fatalf("expected clusterB to hold only b-only, got %v", got)
	}
}

var errUnknownSnapshot = errors.New("unknown snapshot")

// fakeSnapshotSource serves the metadata of the metadata snapshots by ID.
type fakeSnapshotSource struct {
	metadata map[string]klioclient.BackupMetadata
}

func (f *fakeSnapshotSource) ListSnapshots(
	_ context.Context, _ map[string]string, _ kopia.LogFunc,
) ([]kopia.Manifest, error) {
	return nil, nil
}

func (f *fakeSnapshotSource) RestoreSingleFile(
	_ context.Context, snapshotID string, _ string, _ kopia.LogFunc,
) ([]byte, error) {
	metadata, ok := f.metadata[snapshotID]
	if !ok {
		return nil, errUnknownSnapshot
	}

	return json.Marshal(metadata)
}

// backupSnapshot returns a snapshot of one part of a backup of cluster-a.
func backupSnapshot(name, content, tablespace string, endTime kopia.UTCTimestamp) kopia.Manifest {
	tags := map[string]string{
		"tag:" + klioclient.BackupNameTagName:    name,
		"tag:" + klioclient.BackupContentTagName: content,
	}
	if tablespace != "" {
		tags["tag:"+klioclient.TablespaceNameTagName] = tablespace
	}

	return kopia.Manifest{
		ID:      name + "-" + content,
		Source:  kopia.SourceInfo{Host: "cluster-a", Path: "/" + content},
		EndTime: endTime,
		Tags:    tags,
	}
}

func TestCollectBackupMetricsCountsOnlyCompleteBackups(t *testing.T) {
	withTablespace := []klioclient.TablespaceLayout{{Name: "tbs1"}}

	var snapshots []kopia.Manifest
	// b1 is complete, b2 misses its pgdata, b3 misses its tablespace, b4 is
	// complete, and b5 misses its tablespace.
	for i, name := range []string{"b1", "b2", "b3", "b4", "b5"} {
		endTime := kopia.UTCTimestamp(10 * (i + 1))
		snapshots = append(snapshots, backupSnapshot(name, "metadata", "", endTime))
		snapshots = append(snapshots, backupSnapshot(name, "controldata", "", endTime))
		if name != "b2" {
			snapshots = append(snapshots, backupSnapshot(name, "pgdata", "", endTime))
		}
		if name != "b3" && name != "b5" {
			snapshots = append(snapshots, backupSnapshot(name, "tablespace", "tbs1", endTime))
		}
	}

	metadata := make(map[string]klioclient.BackupMetadata)
	for _, name := range []string{"b1", "b2", "b3", "b4", "b5"} {
		metadata[name+"-metadata"] = klioclient.BackupMetadata{
			Name: name, ClusterName: "cluster-a", Tablespaces: withTablespace,
		}
	}

	c := newTestCollector(opentelemetry.Tier1)
	c.kopia = &fakeSnapshotSource{metadata: metadata}

	entries := c.collectBackupMetrics(context.Background(), snapshots)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	// b2 is not counted, having no pgdata. b3 and b5 are, as only the
	// latest and oldest backups are checked for their tablespaces.
	if e.count != 4 {
		t.Fatalf("expected count=4, got %d", e.count)
	}
	if e.latest.Name != "b4" {
		t.Fatalf("expected latest=b4, the newest complete backup, got %s", e.latest.Name)
	}
	if e.oldest.Name != "b1" {
		t.Fatalf("expected oldest=b1, got %s", e.oldest.Name)
	}
}
