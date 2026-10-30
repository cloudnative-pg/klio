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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

var errUnknownSnapshot = errors.New("unknown snapshot")

// fakeMetadataStore serves a fixed set of manifests, filtered by tag as
// Kopia does, and the metadata of the metadata snapshots.
type fakeMetadataStore struct {
	entries  []kopia.Manifest
	metadata map[string]klioclient.BackupMetadata
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
		name     string
		metadata *klioclient.BackupMetadata
		present  map[string]struct{}
		want     []string
	}{
		{
			name:     "complete without tablespaces",
			metadata: &klioclient.BackupMetadata{},
			present:  map[string]struct{}{"metadata": {}, "pgdata": {}, "controldata": {}},
		},
		{
			name:     "complete with tablespaces",
			metadata: withTablespaces,
			present: map[string]struct{}{
				"metadata": {}, "pgdata": {}, "controldata": {},
				"tablespace/tbs1": {}, "tablespace/tbs2": {},
			},
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
			want: []string{"tablespace/tbs1"},
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
		})
	}
}

func TestListBackupsByCompleteness(t *testing.T) {
	complete, incomplete, err := listBackups(context.Background(), newFakeStore(), "c1")
	require.NoError(t, err)

	assert.Equal(t, []string{"b1"}, backupNamesOf(complete))
	assert.ElementsMatch(t, []string{"b2", "b3"}, backupNamesOf(incomplete))
}

func TestListBackupsOtherHost(t *testing.T) {
	complete, incomplete, err := listBackups(context.Background(), newFakeStore(), "c2")
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

			incomplete, ok := errors.AsType[IncompleteBackupError](err)
			if tt.wantMissing == nil {
				assert.False(t, ok, "unexpected incomplete backup error %v", err)
				_, ok = errors.AsType[NoBackupFoundError](err)
				require.True(t, ok, "unexpected error %v", err)
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

// at returns m started at t.
func at(m kopia.Manifest, t string) kopia.Manifest {
	ts, err := time.Parse(time.RFC3339, t)
	if err != nil {
		panic(err)
	}
	m.StartTime = kopia.UTCTimestamp(ts.UnixNano())

	return m
}

// newIncompleteFakeStore returns a store holding, on host c1: b1 complete,
// b2 missing its tablespace, b3 with only its metadata, and b4 with only
// its pgdata and a checkpoint of its controldata; on host c2: b1 with only
// its pgdata.
func newIncompleteFakeStore() *fakeMetadataStore {
	withTablespace := []klioclient.TablespaceLayout{{Name: "tbs1"}}

	checkpoint := at(part("c1", "b4", "controldata", ""), "2026-01-04T00:00:05Z")
	checkpoint.IncompleteReason = "checkpoint"

	return &fakeMetadataStore{
		entries: []kopia.Manifest{
			at(part("c1", "b1", "metadata", ""), "2026-01-01T00:00:10Z"),
			at(part("c1", "b1", "pgdata", ""), "2026-01-01T00:00:00Z"),
			at(part("c1", "b1", "controldata", ""), "2026-01-01T00:00:05Z"),
			at(part("c1", "b2", "metadata", ""), "2026-01-02T00:00:10Z"),
			at(part("c1", "b2", "pgdata", ""), "2026-01-02T00:00:00Z"),
			at(part("c1", "b2", "controldata", ""), "2026-01-02T00:00:05Z"),
			at(part("c1", "b3", "metadata", ""), "2026-01-03T00:00:10Z"),
			at(part("c1", "b4", "pgdata", ""), "2026-01-04T00:00:00Z"),
			checkpoint,
			at(part("c2", "b1", "pgdata", ""), "2026-01-01T00:00:00Z"),
		},
		metadata: map[string]klioclient.BackupMetadata{
			"c1/b1/metadata": {Name: "b1", ClusterName: "c1"},
			"c1/b2/metadata": {Name: "b2", ClusterName: "c1", Tablespaces: withTablespace},
			"c1/b3/metadata": {Name: "b3", ClusterName: "c1"},
		},
	}
}

func TestListIncompleteBackups(t *testing.T) {
	got, err := listIncompleteBackups(context.Background(), newIncompleteFakeStore(), "")
	require.NoError(t, err)

	assert.Equal(t, []klioclient.IncompleteBackup{
		{
			ClusterName:     "c1",
			BackupName:      "b2",
			StartedAt:       "2026-01-02T00:00:00Z",
			MetadataPresent: true,
			MissingParts:    []string{"tablespace/tbs1"},
		},
		{
			ClusterName:     "c1",
			BackupName:      "b3",
			StartedAt:       "2026-01-03T00:00:10Z",
			MetadataPresent: true,
			MissingParts:    []string{"pgdata", "controldata"},
		},
		{
			ClusterName:  "c1",
			BackupName:   "b4",
			StartedAt:    "2026-01-04T00:00:00Z",
			MissingParts: []string{},
			Latest:       true,
		},
		{
			ClusterName:  "c2",
			BackupName:   "b1",
			StartedAt:    "2026-01-01T00:00:00Z",
			MissingParts: []string{},
			Latest:       true,
		},
	}, got)
}

func TestListIncompleteBackupsFiltersByHostname(t *testing.T) {
	got, err := listIncompleteBackups(context.Background(), newIncompleteFakeStore(), "c2")
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.Equal(t, "c2", got[0].ClusterName)
	assert.Equal(t, "b1", got[0].BackupName)
}

func TestListIncompleteBackupsLatestIsCompleteBackup(t *testing.T) {
	// c1: b1 with only its pgdata, then b2 complete: b1 is not the latest
	// attempt, so it can never be a live upload.
	store := &fakeMetadataStore{
		entries: []kopia.Manifest{
			at(part("c1", "b1", "pgdata", ""), "2026-01-01T00:00:00Z"),
			at(part("c1", "b2", "metadata", ""), "2026-01-02T00:00:10Z"),
			at(part("c1", "b2", "pgdata", ""), "2026-01-02T00:00:00Z"),
			at(part("c1", "b2", "controldata", ""), "2026-01-02T00:00:05Z"),
		},
		metadata: map[string]klioclient.BackupMetadata{
			"c1/b2/metadata": {Name: "b2", ClusterName: "c1"},
		},
	}

	got, err := listIncompleteBackups(context.Background(), store, "")
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.Equal(t, "b1", got[0].BackupName)
	assert.False(t, got[0].Latest)
}

func TestListIncompleteBackupsLatestIgnoresEntryOrder(t *testing.T) {
	// b2's checkpoint is listed before its pgdata: the later entry lowers
	// the group's start time, which must still be the latest of the host.
	checkpoint := at(part("c1", "b2", "controldata", ""), "2026-01-02T00:00:05Z")
	checkpoint.IncompleteReason = "checkpoint"
	store := &fakeMetadataStore{
		entries: []kopia.Manifest{
			checkpoint,
			at(part("c1", "b1", "pgdata", ""), "2026-01-01T00:00:00Z"),
			at(part("c1", "b2", "pgdata", ""), "2026-01-02T00:00:00Z"),
		},
	}

	got, err := listIncompleteBackups(context.Background(), store, "")
	require.NoError(t, err)

	require.Len(t, got, 2)
	assert.False(t, got[0].Latest, "b1")
	assert.True(t, got[1].Latest, "b2")
}
