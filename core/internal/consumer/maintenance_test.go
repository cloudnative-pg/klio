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

package consumer

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

func snapshot(path, backup, content, tablespace string) kopia.Manifest {
	return kopia.Manifest{
		Source: kopia.SourceInfo{Host: "c", UserName: "klio", Path: path},
		Tags: map[string]string{
			klioclient.BackupNameTagName:     backup,
			klioclient.BackupContentTagName:  content,
			klioclient.TablespaceNameTagName: tablespace,
		},
	}
}

func TestKeepUntilOnTier2(t *testing.T) {
	tier1 := []kopia.Manifest{
		snapshot("/pgdata", "b1", "pgdata", ""),
		snapshot("/pgdata_meta", "b1", "metadata", ""),
		snapshot("/tbs/a", "b1", "tablespace", "a"),
		snapshot("/pgdata", "b2", "pgdata", ""),
		snapshot("/pgdata_meta", "b2", "metadata", ""),
		snapshot("/tbs/a", "b2", "tablespace", "a"),
		snapshot("/pgdata", "b3", "pgdata", ""),
		snapshot("/pgdata_meta", "b3", "metadata", ""),
		snapshot("/pgdata", "b0", "pgdata", ""),
		snapshot("/pgdata_meta", "b0", "metadata", ""),
	}
	// b1 fully relayed, b2 has its metadata but not its tablespace on tier2,
	// b3 (newest) not relayed yet, b0 (oldest) absent: relayed with b1 and
	// deleted by tier2 retention since.
	tier2 := []kopia.Manifest{
		snapshot("/pgdata", "b1", "pgdata", ""),
		snapshot("/pgdata_meta", "b1", "metadata", ""),
		snapshot("/tbs/a", "b1", "tablespace", "a"),
		snapshot("/pgdata", "b2", "pgdata", ""),
		snapshot("/pgdata_meta", "b2", "metadata", ""),
	}
	catalog := klioclient.BackupList{
		{Name: "b0", StartedAt: 50},
		{Name: "b1", StartedAt: 100},
		{Name: "b2", StartedAt: 200},
		{Name: "b3", StartedAt: 300},
	}
	backup := func(name string) *klioclient.BackupMetadata {
		for i := range catalog {
			if catalog[i].Name == name {
				b := catalog[i]

				return &b
			}
		}

		return &klioclient.BackupMetadata{Name: name}
	}

	skipped := backup("b3")
	skipped.SetAnnotation(klioclient.Tier2RelayAnnotationName, klioclient.Tier2RelaySkipped)

	tests := []struct {
		name   string
		tier2  []kopia.Manifest
		backup *klioclient.BackupMetadata
		want   bool
	}{
		{name: "complete on tier2 is deletable", tier2: tier2, backup: backup("b1"), want: false},
		{name: "tablespace missing on tier2 is kept", tier2: tier2, backup: backup("b2"), want: true},
		{name: "newer than any relayed backup is kept", tier2: tier2, backup: backup("b3"), want: true},
		{name: "older than a relayed backup and gone from tier2 is deletable", tier2: tier2, backup: backup("b0")},
		{name: "unknown to tier1 has nothing to protect", tier2: tier2, backup: backup("b4"), want: false},
		{name: "never meant to reach tier2 is not waited for", tier2: tier2, backup: skipped, want: false},
		{name: "nothing relayed keeps the oldest", tier2: nil, backup: backup("b0"), want: true},
		{name: "nothing relayed keeps the newest", tier2: nil, backup: backup("b3"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keepUntilOnTier2(tier1, tt.tier2, catalog)(tt.backup); got != tt.want {
				t.Errorf("keep(%s) = %v, want %v", tt.backup.Name, got, tt.want)
			}
		})
	}
}

// orphanSnapshot is a tier1 part with no metadata snapshot, started at the
// given Unix time. Its ID is derived from the backup name and content so
// tests can assert exactly which manifest got deleted.
func orphanSnapshot(backup, content string, startedAt int64) kopia.Manifest {
	m := snapshot("/"+content, backup, content, "")
	m.ID = backup + "/" + content
	m.StartTime = kopia.UTCTimestamp(time.Unix(startedAt, 0).UnixNano())

	return m
}

func TestDeleteAbandonedOrphans(t *testing.T) {
	knownGood := klioclient.BackupList{
		{Name: "good1", StartedAt: 100},
		{Name: "good2", StartedAt: 300},
	}

	tests := []struct {
		name         string
		tier1Backups klioclient.BackupList
		snapshots    []kopia.Manifest
		failOn       string
		wantDeleted  []string
		wantErr      error
	}{
		{
			name:         "orphan older than the newest known-good backup is deleted",
			tier1Backups: knownGood,
			snapshots: []kopia.Manifest{
				orphanSnapshot("orphan", "pgdata", 50),
				orphanSnapshot("orphan", "control", 60),
			},
			wantDeleted: []string{"orphan/control", "orphan/pgdata"},
		},
		{
			name:         "orphan newer than the newest known-good backup is kept",
			tier1Backups: knownGood,
			snapshots: []kopia.Manifest{
				orphanSnapshot("orphan", "pgdata", 400),
			},
		},
		{
			name:         "orphan as new as the newest known-good backup is kept",
			tier1Backups: knownGood,
			snapshots: []kopia.Manifest{
				orphanSnapshot("orphan", "pgdata", 300),
			},
		},
		{
			name:         "a known backup's own parts are never touched",
			tier1Backups: knownGood,
			snapshots: []kopia.Manifest{
				orphanSnapshot("good1", "pgdata", 100),
			},
		},
		{
			name:         "no known-good backup yet keeps every orphan",
			tier1Backups: nil,
			snapshots: []kopia.Manifest{
				orphanSnapshot("orphan", "pgdata", 1),
			},
		},
		{
			name:         "a delete failure is reported",
			tier1Backups: knownGood,
			snapshots: []kopia.Manifest{
				orphanSnapshot("orphan", "pgdata", 50),
			},
			failOn:  "orphan/pgdata",
			wantErr: errFakeDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeRetentionClient{failOn: tt.failOn}
			err := deleteAbandonedOrphans(context.Background(), client, "cluster", tt.snapshots, tt.tier1Backups)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("deleteAbandonedOrphans() error = %v, want %v", err, tt.wantErr)
			}

			slices.Sort(client.deleted)
			if !slices.Equal(client.deleted, tt.wantDeleted) {
				t.Errorf("deleted = %v, want %v", client.deleted, tt.wantDeleted)
			}
		})
	}
}

func TestClampWAL(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		frontier  string
		expected  string
	}{
		{
			name:      "empty frontier skips deletion",
			requested: "000000010000000000000005",
			frontier:  "",
			expected:  "",
		},
		{
			name:      "frontier older than requested clamps to frontier",
			requested: "00000001000000000000000A",
			frontier:  "000000010000000000000005",
			expected:  "000000010000000000000005",
		},
		{
			name:      "frontier newer than requested keeps requested",
			requested: "000000010000000000000005",
			frontier:  "00000001000000000000000A",
			expected:  "000000010000000000000005",
		},
		{
			name:      "frontier equal to requested keeps requested",
			requested: "000000010000000000000007",
			frontier:  "000000010000000000000007",
			expected:  "000000010000000000000007",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampWAL(tt.requested, tt.frontier)
			if got != tt.expected {
				t.Errorf("clampWAL(%q, %q) = %q, want %q", tt.requested, tt.frontier, got, tt.expected)
			}
		})
	}
}
