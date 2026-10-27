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
	"context"
	"encoding/json"
	"fmt"

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

// getMetadata returns the metadata of the named backup of hostname.
func getMetadata(
	ctx context.Context,
	store MetadataStore,
	hostname string,
	name string,
) (*klioclient.BackupMetadata, error) {
	contextLogger := log.FromContext(ctx)
	entries, err := store.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName: name,
	}, contextLogger.Debug)
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	parts := NewBackupParts(entries)

	for _, entry := range entries {
		if entry.Source.Host != hostname || TagValue(entry, klioclient.BackupContentTagName) != "metadata" {
			continue
		}

		metadata, err := RestoreMetadata(ctx, store, entry.ID)
		if err != nil {
			return nil, err
		}

		if missing := parts.Missing(hostname, name, metadata); len(missing) > 0 {
			return nil, newIncompleteBackupError(hostname, name, missing)
		}

		return metadata, nil
	}

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
// (of every host when empty).
func listBackups(
	ctx context.Context,
	store MetadataStore,
	hostname string,
) (klioclient.BackupList, klioclient.BackupList, error) {
	contextLogger := log.FromContext(ctx)

	// Unfiltered: the completeness check needs every content type, not
	// just the metadata snapshots.
	entries, err := store.ListSnapshots(ctx, nil, contextLogger.Info)
	if err != nil {
		return nil, nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	parts := NewBackupParts(entries)

	var complete, incomplete klioclient.BackupList
	for _, entry := range entries {
		if !isMetadataOf(entry, hostname) {
			continue
		}

		host := entry.Source.Host
		name := TagValue(entry, klioclient.BackupNameTagName)
		metadata, err := RestoreMetadata(ctx, store, entry.ID)
		if err != nil {
			contextLogger.Error(err, "Error while decoding backup metadata, skipping", "id", entry.ID)

			continue
		}

		if missing := parts.Missing(host, name, metadata); len(missing) > 0 {
			contextLogger.Debug("Backup is incomplete on this tier",
				"hostname", host, "backupName", name, "missing", missing)
			incomplete = append(incomplete, *metadata)

			continue
		}

		complete = append(complete, *metadata)
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
