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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

var errUnknownSnapshot = errors.New("unknown snapshot")

// fakeMetadataStore serves a fixed set of manifests, filtered by tag as
// Kopia does, and the metadata of the metadata snapshots. It records the
// IDs of the restored snapshots.
type fakeMetadataStore struct {
	entries  []kopia.Manifest
	metadata map[string]klioclient.BackupMetadata
	restored []string
}

func (f *fakeMetadataStore) ListSnapshots(
	_ context.Context,
	tags map[string]string,
	_ kopia.LogFunc,
) ([]kopia.Manifest, error) {
	var result []kopia.Manifest
	for _, entry := range f.entries {
		matches := true
		for k, v := range tags {
			if TagValue(entry, k) != v {
				matches = false
			}
		}
		if matches {
			result = append(result, entry)
		}
	}

	return result, nil
}

func (f *fakeMetadataStore) RestoreSingleFile(
	_ context.Context,
	snapshotID string,
	_ string,
	_ kopia.LogFunc,
) ([]byte, error) {
	metadata, ok := f.metadata[snapshotID]
	if !ok {
		return nil, errUnknownSnapshot
	}
	f.restored = append(f.restored, snapshotID)

	return json.Marshal(metadata)
}

func part(host, name, content, tablespace string) kopia.Manifest {
	tags := map[string]string{
		tagKeyPrefix + klioclient.BackupNameTagName:    name,
		tagKeyPrefix + klioclient.BackupContentTagName: content,
	}
	if tablespace != "" {
		tags[tagKeyPrefix+klioclient.TablespaceNameTagName] = tablespace
	}

	return kopia.Manifest{
		ID:     host + "/" + name + "/" + content + tablespace,
		Source: kopia.SourceInfo{Host: host},
		Tags:   tags,
	}
}

// newFakeStore returns a store holding, on host c1: b1 complete with
// tablespace tbs1, b2 missing tbs1, and b3 with only its metadata.
func newFakeStore() *fakeMetadataStore {
	withTablespace := []klioclient.TablespaceLayout{{Name: "tbs1"}}

	return &fakeMetadataStore{
		entries: []kopia.Manifest{
			part("c1", "b1", "metadata", ""),
			part("c1", "b1", "pgdata", ""),
			part("c1", "b1", "controldata", ""),
			part("c1", "b1", "tablespace", "tbs1"),
			part("c1", "b2", "metadata", ""),
			part("c1", "b2", "pgdata", ""),
			part("c1", "b2", "controldata", ""),
			part("c1", "b3", "metadata", ""),
		},
		metadata: map[string]klioclient.BackupMetadata{
			"c1/b1/metadata": {Name: "b1", Tablespaces: withTablespace},
			"c1/b2/metadata": {Name: "b2", Tablespaces: withTablespace},
			"c1/b3/metadata": {Name: "b3"},
		},
	}
}

func backupNamesOf(backups klioclient.BackupList) []string {
	names := make([]string, 0, len(backups))
	for _, b := range backups {
		names = append(names, b.Name)
	}

	return names
}

func TestNewBackupParts(t *testing.T) {
	incomplete := part("c1", "b1", "controldata", "")
	incomplete.IncompleteReason = "checkpoint"

	untagged := kopia.Manifest{Source: kopia.SourceInfo{Host: "c1"}}

	got := NewBackupParts([]kopia.Manifest{
		part("c1", "b1", "metadata", ""),
		part("c1", "b1", "pgdata", ""),
		part("c1", "b1", "tablespace", "tbs1"),
		incomplete,
		untagged,
		part("c1", "b2", "pgdata", ""),
		part("c2", "b1", "pgdata", ""),
	})

	assert.Equal(t, BackupParts{
		{host: "c1", name: "b1"}: {
			"metadata":        {},
			"pgdata":          {},
			"tablespace/tbs1": {},
		},
		{host: "c1", name: "b2"}: {
			"pgdata": {},
		},
		{host: "c2", name: "b1"}: {
			"pgdata": {},
		},
	}, got)
}

func TestBackupPartsMissing(t *testing.T) {
	withTablespaces := &klioclient.BackupMetadata{
		Tablespaces: []klioclient.TablespaceLayout{{Name: "tbs1"}, {Name: "tbs2"}},
	}

	tests := []struct {
		name         string
		metadata     *klioclient.BackupMetadata
		present      map[string]struct{}
		want         []string
		wantDataPart bool
	}{
		{
			name:         "complete without tablespaces",
			metadata:     &klioclient.BackupMetadata{},
			present:      map[string]struct{}{"metadata": {}, "pgdata": {}, "controldata": {}},
			wantDataPart: true,
		},
		{
			name:     "complete with tablespaces",
			metadata: withTablespaces,
			present: map[string]struct{}{
				"metadata": {}, "pgdata": {}, "controldata": {},
				"tablespace/tbs1": {}, "tablespace/tbs2": {},
			},
			wantDataPart: true,
		},
		{
			name:     "only metadata",
			metadata: withTablespaces,
			present:  map[string]struct{}{"metadata": {}},
			want:     []string{"pgdata", "controldata", "tablespace/tbs1", "tablespace/tbs2"},
		},
		{
			name:     "missing one tablespace",
			metadata: withTablespaces,
			present: map[string]struct{}{
				"metadata": {}, "pgdata": {}, "controldata": {}, "tablespace/tbs2": {},
			},
			want:         []string{"tablespace/tbs1"},
			wantDataPart: true,
		},
		{
			name:     "missing control data",
			metadata: &klioclient.BackupMetadata{},
			present:  map[string]struct{}{"metadata": {}, "pgdata": {}},
			want:     []string{"controldata"},
		},
		{
			name:     "no snapshots",
			metadata: &klioclient.BackupMetadata{},
			want:     []string{"pgdata", "controldata"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := BackupParts{{host: "c1", name: "b1"}: tt.present}
			assert.Equal(t, tt.want, parts.Missing("c1", "b1", tt.metadata))
			assert.Equal(t, tt.wantDataPart, parts.HasDataParts("c1", "b1"))
		})
	}
}

func TestListBackupsCompleteOnly(t *testing.T) {
	store := newFakeStore()

	complete, incomplete, err := listBackups(context.Background(), store, "c1", false)
	require.NoError(t, err)

	assert.Equal(t, []string{"b1"}, backupNamesOf(complete))
	assert.Empty(t, incomplete)
	// b3 has no pgdata or control data, so its metadata is never read.
	assert.ElementsMatch(t, []string{"c1/b1/metadata", "c1/b2/metadata"}, store.restored)
}

func TestListBackupsByCompleteness(t *testing.T) {
	complete, incomplete, err := listBackups(context.Background(), newFakeStore(), "c1", true)
	require.NoError(t, err)

	assert.Equal(t, []string{"b1"}, backupNamesOf(complete))
	assert.ElementsMatch(t, []string{"b2", "b3"}, backupNamesOf(incomplete))
}

func TestListBackupsOtherHost(t *testing.T) {
	complete, incomplete, err := listBackups(context.Background(), newFakeStore(), "c2", true)
	require.NoError(t, err)

	assert.Empty(t, complete)
	assert.Empty(t, incomplete)
}

func TestGetMetadata(t *testing.T) {
	tests := []struct {
		name        string
		backup      string
		wantErr     string
		wantMissing []string
	}{
		{name: "complete backup", backup: "b1"},
		{name: "missing tablespace", backup: "b2", wantMissing: []string{"tablespace/tbs1"}},
		{name: "only metadata", backup: "b3", wantMissing: []string{"pgdata", "controldata"}},
		{name: "unknown backup", backup: "b4", wantErr: `backup "b4" for host "c1" not found`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := getMetadata(context.Background(), newFakeStore(), "c1", tt.backup)

			if tt.wantErr == "" && tt.wantMissing == nil {
				require.NoError(t, err)
				assert.Equal(t, tt.backup, metadata.Name)

				return
			}

			// Both cases must be a NoBackupFoundError, so MultiConnection
			// falls back to tier2.
			_, ok := errors.AsType[NoBackupFoundError](err)
			require.True(t, ok, "unexpected error %v", err)

			incomplete, ok := errors.AsType[IncompleteBackupError](err)
			if tt.wantMissing == nil {
				assert.False(t, ok, "unexpected incomplete backup error %v", err)
			} else {
				require.True(t, ok, "unexpected error %v", err)
				assert.Equal(t, tt.wantMissing, incomplete.missingParts)
			}
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}
