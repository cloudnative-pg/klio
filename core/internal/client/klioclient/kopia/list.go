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

package kopia

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// tagKeyPrefix is the prefix Kopia adds to the keys of user-defined tags
// in the snapshot manifests.
const tagKeyPrefix = "tag:"

// MetadataStore is the subset of the Kopia client that reading the backup
// metadata needs.
type MetadataStore interface {
	ListSnapshots(ctx context.Context, tags map[string]string, logFn kopia.LogFunc) ([]kopia.Manifest, error)
	RestoreSingleFile(ctx context.Context, snapshotID string, fileName string, logFn kopia.LogFunc) ([]byte, error)
}

// GetMetadata implements the BackupRestoreSupport interface. A backup
// whose snapshots are not all present on this tier is reported as not
// found, with the missing parts in the error.
func (s *Connection) GetMetadata(
	ctx context.Context,
	hostname string,
	name string,
) (*klioclient.BackupMetadata, error) {
	return getMetadata(ctx, s.kopia, hostname, name)
}

// getMetadata returns the metadata of the named backup of hostname, with
// the same completeness rule as listBackups: a backup is found only when
// every snapshot it needs is on this tier.
func getMetadata(
	ctx context.Context,
	store MetadataStore,
	hostname string,
	name string,
) (*klioclient.BackupMetadata, error) {
	contextLogger := log.FromContext(ctx)
	// Every snapshot of the backup, whatever its content: the completeness
	// check below needs them all, not just the metadata one.
	entries, err := store.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName: name,
	}, contextLogger.Debug)
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	parts := NewBackupParts(entries)

	for _, entry := range entries {
		if !isMetadataOf(entry, hostname) {
			continue
		}

		metadata, err := RestoreMetadata(ctx, store, entry.ID)
		if err != nil {
			return nil, err
		}

		// Found but not restorable: the error names what is missing.
		if missing := parts.Missing(hostname, name, metadata); len(missing) > 0 {
			return nil, newIncompleteBackupError(hostname, name, missing)
		}

		return metadata, nil
	}

	// No metadata snapshot: never finished uploading, or never existed.
	return nil, newNoBackupFoundError(hostname, name)
}

// ListBackups lists the backups in the repository whose snapshots are all
// present on this tier. A backup still being relayed, or left partially
// deleted, is not listed.
func (s *Connection) ListBackups(ctx context.Context, hostname string) (klioclient.BackupList, error) {
	complete, _, err := listBackups(ctx, s.kopia, hostname)

	return complete, err
}

// ListBackupsByCompleteness lists every backup in the repository that has a
// metadata snapshot, split into the ones whose snapshots are all present on
// this tier and the ones missing some of them.
func (s *Connection) ListBackupsByCompleteness(
	ctx context.Context,
	hostname string,
) (klioclient.BackupList, klioclient.BackupList, error) {
	return listBackups(ctx, s.kopia, hostname)
}

// listBackups returns the complete and the incomplete backups of hostname
// (of every host when empty), among the ones with a metadata snapshot.
func listBackups(
	ctx context.Context,
	store MetadataStore,
	hostname string,
) (klioclient.BackupList, klioclient.BackupList, error) {
	contextLogger := log.FromContext(ctx)

	scanned, err := scanBackups(ctx, store, hostname)
	if err != nil {
		return nil, nil, err
	}

	var complete, incomplete klioclient.BackupList
	for _, b := range scanned {
		switch {
		case b.Metadata == nil:
		case len(b.Missing) > 0:
			contextLogger.Debug("Backup is incomplete on this tier",
				"hostname", b.Host, "backupName", b.Name, "missing", b.Missing)
			incomplete = append(incomplete, *b.Metadata)
		default:
			complete = append(complete, *b.Metadata)
		}
	}

	return complete, incomplete, nil
}

// isMetadataOf reports whether entry is a metadata snapshot of hostname, or
// of any host when hostname is empty.
func isMetadataOf(entry kopia.Manifest, hostname string) bool {
	if hostname != "" && entry.Source.Host != hostname {
		return false
	}

	return TagValue(entry, klioclient.BackupContentTagName) == "metadata"
}

// TagValue returns the value of the user-defined tag name of a manifest.
func TagValue(entry kopia.Manifest, name string) string {
	return entry.Tags[tagKeyPrefix+name]
}

// hostBackupName identifies one backup on one cluster.
type hostBackupName struct {
	host string
	name string
}

// BackupParts records, for each backup of each cluster, the parts that have
// a complete snapshot. A part is named after its content type, followed by
// the tablespace name for tablespaces.
type BackupParts map[hostBackupName]map[string]struct{}

// NewBackupParts builds the BackupParts of the backups in entries.
func NewBackupParts(entries []kopia.Manifest) BackupParts {
	result := make(BackupParts)

	for _, entry := range entries {
		name := TagValue(entry, klioclient.BackupNameTagName)
		if name == "" || entry.IncompleteReason != "" {
			continue
		}

		key := hostBackupName{host: entry.Source.Host, name: name}
		if result[key] == nil {
			result[key] = make(map[string]struct{})
		}

		part := TagValue(entry, klioclient.BackupContentTagName)
		if part == "tablespace" {
			part += "/" + TagValue(entry, klioclient.TablespaceNameTagName)
		}
		result[key][part] = struct{}{}
	}

	return result
}

// Missing returns the parts that the backup described by metadata expects
// but has no snapshot of. The metadata snapshot itself is assumed present.
func (p BackupParts) Missing(hostname, name string, metadata *klioclient.BackupMetadata) []string {
	present := p[hostBackupName{host: hostname, name: name}]

	expected := make([]string, 0, 2+len(metadata.Tablespaces))
	expected = append(expected, "pgdata", "controldata")

	for _, tbl := range metadata.Tablespaces {
		expected = append(expected, "tablespace/"+tablespaceTagValue(tbl))
	}

	var missing []string
	for _, part := range expected {
		if _, ok := present[part]; !ok {
			missing = append(missing, part)
		}
	}

	return missing
}

// RestoreMetadata restores the metadata stored in a snapshot with the
// given ID.
func RestoreMetadata(
	ctx context.Context,
	store MetadataStore,
	snapshotID string,
) (*klioclient.BackupMetadata, error) {
	contextLogger := log.FromContext(ctx)
	manifestContent, err := store.RestoreSingleFile(ctx, snapshotID, "metadata.json", contextLogger.Info)
	if err != nil {
		return nil, fmt.Errorf("restoring backup metadata: %w", err)
	}

	var result klioclient.BackupMetadata
	if err := json.Unmarshal(manifestContent, &result); err != nil {
		return nil, fmt.Errorf("cannot decode JSON backup metadata: %w", err)
	}

	return &result, nil
}

// ListIncompleteBackups lists the backups of hostname (of every host when
// empty) that are not fully present on this tier.
func (s *Connection) ListIncompleteBackups(
	ctx context.Context,
	hostname string,
) ([]klioclient.IncompleteBackup, error) {
	return listIncompleteBackups(ctx, s.kopia, hostname)
}

// listIncompleteBackups returns the backups of hostname (of every host when
// empty) that have no metadata snapshot or miss a part the metadata
// expects.
func listIncompleteBackups(
	ctx context.Context,
	store MetadataStore,
	hostname string,
) ([]klioclient.IncompleteBackup, error) {
	scanned, err := scanBackups(ctx, store, hostname)
	if err != nil {
		return nil, err
	}

	var result []klioclient.IncompleteBackup
	for _, b := range scanned {
		if b.Metadata != nil && len(b.Missing) == 0 {
			continue
		}

		missing := b.Missing
		if b.Metadata == nil {
			// Nothing is known of what the backup expected.
			missing = []string{}
		}

		result = append(result, klioclient.IncompleteBackup{
			ClusterName:     b.Host,
			BackupName:      b.Name,
			StartedAt:       b.StartedAt,
			MetadataPresent: b.Metadata != nil,
			MissingParts:    missing,
			Latest:          b.Latest,
		})
	}

	return result, nil
}

// ScannedBackup is one backup of one host, as found in the repository.
type ScannedBackup struct {
	Host string
	Name string

	// StartedAt is the earliest start time of the backup's snapshots, Kopia
	// checkpoints included.
	StartedAt string

	// Latest reports whether this is the latest started backup of its host.
	Latest bool

	// MetadataID is the ID of the metadata snapshot, empty when missing.
	MetadataID string

	// Metadata is the backup metadata, nil when its snapshot is missing.
	Metadata *klioclient.BackupMetadata

	// Missing lists the parts Metadata expects that have no snapshot.
	Missing []string
}

// MetadataLoader reads the backup metadata stored in a metadata snapshot.
type MetadataLoader func(ctx context.Context, snapshotID string) (*klioclient.BackupMetadata, error)

// backupGroup tracks the snapshots seen for one backup of one host.
type backupGroup struct {
	startedAt  string
	metadataID string
}

// scanBackups lists every snapshot of the repository and scans the backups
// of hostname (of every host when empty).
func scanBackups(ctx context.Context, store MetadataStore, hostname string) ([]ScannedBackup, error) {
	// Unfiltered: the completeness check needs every content type, not
	// just the metadata snapshots.
	entries, err := store.ListSnapshots(ctx, nil, log.FromContext(ctx).Info)
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	load := func(ctx context.Context, snapshotID string) (*klioclient.BackupMetadata, error) {
		return RestoreMetadata(ctx, store, snapshotID)
	}

	return ScanBackups(ctx, entries, hostname, load), nil
}

// ScanBackups returns every backup of hostname (of every host when empty)
// found among entries, complete or not, sorted by host and start time. The
// metadata of each backup that has one is read with load; a backup whose
// metadata cannot be read is logged and skipped.
func ScanBackups(
	ctx context.Context,
	entries []kopia.Manifest,
	hostname string,
	load MetadataLoader,
) []ScannedBackup {
	contextLogger := log.FromContext(ctx)

	parts := NewBackupParts(entries)
	groups := groupBackups(entries, hostname)

	latestPerHost := make(map[string]string)
	for key, g := range groups {
		latestPerHost[key.host] = max(latestPerHost[key.host], g.startedAt)
	}

	result := make([]ScannedBackup, 0, len(groups))
	for key, g := range groups {
		b := ScannedBackup{
			Host:       key.host,
			Name:       key.name,
			StartedAt:  g.startedAt,
			Latest:     g.startedAt == latestPerHost[key.host],
			MetadataID: g.metadataID,
		}

		if g.metadataID != "" {
			metadata, err := load(ctx, g.metadataID)
			if err != nil {
				contextLogger.Error(err, "Error while decoding backup metadata, skipping", "id", g.metadataID)

				continue
			}

			b.Metadata = metadata
			b.Missing = parts.Missing(key.host, key.name, metadata)
		}

		result = append(result, b)
	}

	slices.SortFunc(result, func(a, b ScannedBackup) int {
		return cmp.Or(cmp.Compare(a.Host, b.Host), cmp.Compare(a.StartedAt, b.StartedAt))
	})

	return result
}

// groupBackups groups the entries of hostname (of every host when empty)
// by backup, tracking each backup's earliest start time and the ID of its
// metadata snapshot, if any.
func groupBackups(entries []kopia.Manifest, hostname string) map[hostBackupName]*backupGroup {
	groups := make(map[hostBackupName]*backupGroup)

	for _, entry := range entries {
		name := TagValue(entry, klioclient.BackupNameTagName)
		if name == "" || (hostname != "" && entry.Source.Host != hostname) {
			continue
		}

		key := hostBackupName{host: entry.Source.Host, name: name}
		g, ok := groups[key]
		if !ok {
			g = &backupGroup{startedAt: entry.StartTime}
			groups[key] = g
		}
		g.startedAt = min(g.startedAt, entry.StartTime)
		if entry.IncompleteReason == "" && TagValue(entry, klioclient.BackupContentTagName) == "metadata" {
			g.metadataID = entry.ID
		}
	}

	return groups
}
