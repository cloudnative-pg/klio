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
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioclientkopia "github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// SnapshotMetricsCollector periodically collects Kopia snapshot metrics.
type SnapshotMetricsCollector struct {
	interval time.Duration
	logger   log.Logger
	stopCh   chan struct{}
	kopia    snapshotSource
	tier     opentelemetry.Tier

	// mu guards cache, which is refreshed by the collection ticker and read by
	// the observable-gauge callback.
	mu    sync.RWMutex
	cache metricsCache

	// metadata holds, by snapshot ID, the backup metadata already restored.
	// A snapshot never changes, so each one is restored only once. Only the
	// collection goroutine uses it.
	metadata map[string]*klioclient.BackupMetadata
}

// metricsCache is the most recently collected metric state. The observable
// callback observes only the series present here, so series for snapshots or
// backups that have been removed disappear from export.
type metricsCache struct {
	snapshots []snapshotCacheEntry
	backups   []backupCacheEntry
}

// snapshotCacheEntry holds the snapshot statistics of a single Kopia source.
type snapshotCacheEntry struct {
	source string
	stats  snapshotStats
}

// backupCacheEntry holds the retention window of the PostgreSQL backups of a
// single cluster: the latest and oldest retained backup metadata and the
// number of backups retained.
type backupCacheEntry struct {
	clusterName string
	count       int64
	latest      klioclient.BackupMetadata
	oldest      klioclient.BackupMetadata
}

type snapshotStats struct {
	oldestSnapshotTimestamp  int64
	latestSnapshotTimestamp  int64
	snapshotCount            int64
	snapshotSize             int64
	latestSnapshotFilesCount int64
	latestSnapshotDirCount   int64
}

// snapshotSource is the subset of the Kopia client the collector needs.
type snapshotSource interface {
	ListSnapshots(ctx context.Context, tags map[string]string, logFn kopia.LogFunc) ([]kopia.Manifest, error)
	RestoreSingleFile(ctx context.Context, snapshotID string, fileName string, logFn kopia.LogFunc) ([]byte, error)
}

// newSnapshotMetricsCollector creates a new snapshot metrics collector for
// the given storage tier. The tier value is attached to every emitted sample
// so tier-1 and tier-2 series can be distinguished by consumers.
func newSnapshotMetricsCollector(
	configPath string, interval time.Duration, tier opentelemetry.Tier, logger log.Logger,
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
		tier: tier,
	}, nil
}

// Start begins the periodic collection of snapshot metrics.
func (c *SnapshotMetricsCollector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.logger.Info("Starting Kopia snapshot metrics collection", "interval", c.interval)
	// Populate the cache before registering the callback so the first
	// collection is not empty.
	c.collectMetrics(ctx)
	unregister := c.registerCallback()
	defer func() { _ = unregister() }()

	// The metrics are collected and cached periodically, and the observable-gauge callback observes the cached state.
	// This ensures that the metrics are up-to-date and consistent, while also allowing for efficient observation
	// without slowing/blocking the collection process.
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

// registerCallback registers the observable-gauge callback against the shared
// server-backup instruments. Each collector (one per tier) registers its own
// callback observing the same instruments with its own `tier` attribute.
func (c *SnapshotMetricsCollector) registerCallback() func() error {
	reg, err := otel.Meter(opentelemetry.Meter).RegisterCallback(
		c.observe,
		opentelemetry.ServerBackup.TotalSnapshots,
		opentelemetry.ServerBackup.OldestSnapshotTimestamp,
		opentelemetry.ServerBackup.LatestSnapshotTimestamp,
		opentelemetry.ServerBackup.LatestSnapshotSize,
		opentelemetry.ServerBackup.LatestSnapshotDirCount,
		opentelemetry.ServerBackup.LatestSnapshotFileCount,
		opentelemetry.ServerBackup.Backups,
		opentelemetry.ServerBackup.LatestBackupStartTime,
		opentelemetry.ServerBackup.LatestBackupEndTime,
		opentelemetry.ServerBackup.LatestBackupStartLSN,
		opentelemetry.ServerBackup.LatestBackupEndLSN,
		opentelemetry.ServerBackup.LatestBackupTimeline,
		opentelemetry.ServerBackup.OldestBackupStartTime,
		opentelemetry.ServerBackup.OldestBackupEndTime,
		opentelemetry.ServerBackup.OldestBackupStartLSN,
		opentelemetry.ServerBackup.OldestBackupEndLSN,
		opentelemetry.ServerBackup.OldestBackupTimeline,
	)
	if err != nil {
		c.logger.Error(err, "Failed to register backup metrics callback")
		return nil
	}

	return reg.Unregister
}

// observe reports the cached snapshot and backup state.
func (c *SnapshotMetricsCollector) observe(_ context.Context, o metric.Observer) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, e := range c.cache.snapshots {
		attrs := metric.WithAttributes(
			opentelemetry.AttributeKeySnapshotSource.Of(e.source),
			c.tier.Attribute(),
		)
		o.ObserveInt64(opentelemetry.ServerBackup.TotalSnapshots, e.stats.snapshotCount, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.OldestSnapshotTimestamp, e.stats.oldestSnapshotTimestamp, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.LatestSnapshotTimestamp, e.stats.latestSnapshotTimestamp, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.LatestSnapshotSize, e.stats.snapshotSize, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.LatestSnapshotDirCount, e.stats.latestSnapshotDirCount, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.LatestSnapshotFileCount, e.stats.latestSnapshotFilesCount, attrs)
	}

	for _, e := range c.cache.backups {
		attrs := metric.WithAttributes(
			opentelemetry.AttributeKeyClusterName.Of(e.clusterName),
			c.tier.Attribute(),
		)
		o.ObserveInt64(opentelemetry.ServerBackup.Backups, e.count, attrs)

		o.ObserveInt64(opentelemetry.ServerBackup.LatestBackupStartTime, e.latest.StartedAt, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.LatestBackupEndTime, e.latest.StoppedAt, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.LatestBackupStartLSN, int64(e.latest.StartLSN), attrs) //nolint:gosec
		o.ObserveInt64(opentelemetry.ServerBackup.LatestBackupEndLSN, int64(e.latest.EndLSN), attrs)     //nolint:gosec
		o.ObserveInt64(opentelemetry.ServerBackup.LatestBackupTimeline, int64(e.latest.Timeline), attrs)

		o.ObserveInt64(opentelemetry.ServerBackup.OldestBackupStartTime, e.oldest.StartedAt, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.OldestBackupEndTime, e.oldest.StoppedAt, attrs)
		o.ObserveInt64(opentelemetry.ServerBackup.OldestBackupStartLSN, int64(e.oldest.StartLSN), attrs) //nolint:gosec
		o.ObserveInt64(opentelemetry.ServerBackup.OldestBackupEndLSN, int64(e.oldest.EndLSN), attrs)     //nolint:gosec
		o.ObserveInt64(opentelemetry.ServerBackup.OldestBackupTimeline, int64(e.oldest.Timeline), attrs)
	}

	return nil
}

// collectMetrics refreshes the metric cache from Kopia.
func (c *SnapshotMetricsCollector) collectMetrics(ctx context.Context) {
	c.logger.Debug("Collecting Kopia snapshot metrics")
	snapshots, err := c.kopia.ListSnapshots(ctx, nil, c.logger.Debug)
	if err != nil {
		// Keep the previous cache so a transient failure does not flap every
		// series to absent.
		c.logger.Warning("Failed to get Kopia snapshots", "error", err)
		return
	}

	snapshotEntries := c.collectSnapshotMetrics(ctx, snapshots)
	backupEntries := c.collectBackupMetrics(ctx, snapshots)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.snapshots = snapshotEntries
	c.cache.backups = backupEntries
}

// collectSnapshotMetrics aggregates snapshot data per source into cache entries.
func (c *SnapshotMetricsCollector) collectSnapshotMetrics(
	ctx context.Context, snapshots []kopia.Manifest,
) []snapshotCacheEntry {
	stats := make(map[kopia.SourceInfo]snapshotStats)
	for _, s := range snapshots {
		snapshotLogger := log.FromContext(ctx).WithValues("snapshotID", s.ID, "source", s.Source)
		snapshotLogger.Debug("Processing snapshot")

		if s.RootEntry == nil {
			snapshotLogger.Debug("Skipping snapshot with nil RootEntry")
			continue
		}

		timestamp := s.EndTime.ToTime().Unix()
		ds := s.RootEntry.DirSummary

		v := stats[s.Source]
		v.update(timestamp, ds)
		stats[s.Source] = v
	}

	entries := make([]snapshotCacheEntry, 0, len(stats))
	for origin, stat := range stats {
		entries = append(entries, snapshotCacheEntry{
			source: origin.String(),
			stats:  stat,
		})
	}

	return entries
}

func (s *snapshotStats) update(timestamp int64, ds *kopia.DirectorySummary) {
	defer func() {
		s.snapshotCount++
	}()

	if s.snapshotCount == 0 {
		s.oldestSnapshotTimestamp = timestamp
		s.latestSnapshotTimestamp = timestamp
		s.snapshotSize = ds.TotalFileSize
		s.latestSnapshotFilesCount = ds.TotalFileCount
		s.latestSnapshotDirCount = ds.TotalDirCount

		return
	}

	if timestamp < s.oldestSnapshotTimestamp {
		s.oldestSnapshotTimestamp = timestamp
	}
	if timestamp > s.latestSnapshotTimestamp {
		s.latestSnapshotTimestamp = timestamp
		s.snapshotSize = ds.TotalFileSize
		s.latestSnapshotFilesCount = ds.TotalFileCount
		s.latestSnapshotDirCount = ds.TotalDirCount
	}
}

// collectBackupMetrics reads the snapshotted PostgreSQL backup metadata and
// builds per-cluster cache entries describing the retention window on this
// tier: the latest and oldest retained backup and the number of backups
// retained. Only complete backups are counted, as ListBackups does, so the
// metrics agree with the backup list. A backup whose metadata cannot be read
// is skipped, as ListBackups does, and retried on the next collection.
func (c *SnapshotMetricsCollector) collectBackupMetrics(
	ctx context.Context, snapshots []kopia.Manifest,
) []backupCacheEntry {
	c.logger.Debug("Collecting PostgreSQL backup metrics")

	parts := klioclientkopia.NewBackupParts(snapshots)
	metadata := make(map[string]*klioclient.BackupMetadata, len(c.metadata))
	var complete []kopia.Manifest
	for _, s := range snapshots {
		if klioclientkopia.TagValue(s, klioclient.BackupContentTagName) != "metadata" {
			continue
		}

		meta, ok := c.metadata[s.ID]
		if !ok {
			var err error
			if meta, err = c.readBackupMetadata(ctx, s.ID); err != nil {
				c.logger.Warning("Failed to read backup metadata", "snapshotID", s.ID, "error", err)
				continue
			}
		}
		metadata[s.ID] = meta

		name := klioclientkopia.TagValue(s, klioclient.BackupNameTagName)
		if missing := parts.Missing(s.Source.Host, name, meta); len(missing) > 0 {
			c.logger.Debug("Skipping incomplete backup", "snapshotID", s.ID, "missing", missing)
			continue
		}
		complete = append(complete, s)
	}
	// Replacing the cache drops the metadata of deleted snapshots.
	c.metadata = metadata

	groups := buildBackupGroups(complete)
	entries := make([]backupCacheEntry, 0, len(groups))
	for _, group := range groups {
		latest, oldest := metadata[group[len(group)-1].ID], metadata[group[0].ID]
		entries = append(entries, backupCacheEntry{
			clusterName: latest.ClusterName,
			count:       int64(len(group)),
			latest:      *latest,
			oldest:      *oldest,
		})
	}

	return entries
}

// buildBackupGroups groups metadata snapshots by source, each group sorted
// by ascending end time.
func buildBackupGroups(snapshots []kopia.Manifest) map[kopia.SourceInfo][]kopia.Manifest {
	groups := make(map[kopia.SourceInfo][]kopia.Manifest)
	for _, s := range snapshots {
		groups[s.Source] = append(groups[s.Source], s)
	}

	for _, group := range groups {
		slices.SortFunc(group, func(a, b kopia.Manifest) int {
			return cmp.Compare(a.EndTime, b.EndTime)
		})
	}

	return groups
}

// readBackupMetadata reads the backup metadata stored in the
// snapshot with the given ID.
func (c *SnapshotMetricsCollector) readBackupMetadata(
	ctx context.Context, snapshotID string,
) (*klioclient.BackupMetadata, error) {
	content, err := c.kopia.RestoreSingleFile(ctx, snapshotID, "metadata.json", c.logger.Debug)
	if err != nil {
		return nil, fmt.Errorf("restoring backup metadata: %w", err)
	}

	var meta klioclient.BackupMetadata
	if err := json.Unmarshal(content, &meta); err != nil {
		return nil, fmt.Errorf("decoding backup metadata: %w", err)
	}

	return &meta, nil
}
