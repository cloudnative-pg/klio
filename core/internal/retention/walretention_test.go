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

package retention

import (
	"context"
	"errors"
	"path"
	"slices"
	"testing"

	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioclientkopia "github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

func TestFindOldestWAL(t *testing.T) {
	tests := []struct {
		name    string
		backups klioclient.BackupList
		want    string
	}{
		{
			name:    "empty list",
			backups: nil,
			want:    "",
		},
		{
			name: "all empty StartWAL",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: ""},
				{Name: "b2", StartWAL: ""},
			},
			want: "",
		},
		{
			name: "single entry",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: "000000010000000000000005"},
			},
			want: "000000010000000000000005",
		},
		{
			name: "already sorted ascending",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: "000000010000000000000001"},
				{Name: "b2", StartWAL: "000000010000000000000005"},
				{Name: "b3", StartWAL: "00000001000000000000000A"},
			},
			want: "000000010000000000000001",
		},
		{
			name: "unsorted, mixed with empty StartWAL entries",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: "000000010000000000000005"},
				{Name: "b2", StartWAL: ""},
				{Name: "b3", StartWAL: "000000010000000000000002"},
				{Name: "b4", StartWAL: "00000001000000000000000A"},
			},
			want: "000000010000000000000002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findOldestWAL(tt.backups); got != tt.want {
				t.Fatalf("findOldestWAL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakePendingWALs is a PendingWALLister returning a fixed answer.
type fakePendingWALs struct {
	oldest string
	err    error
}

func (f fakePendingWALs) OldestPendingWAL(context.Context, string) (string, error) {
	return f.oldest, f.err
}

var errFakePendingWALs = errors.New("queue unavailable")

func TestHoldForPendingWALs(t *testing.T) {
	const firstRequired = "00000001000000000000000A"

	tests := []struct {
		name    string
		pending PendingWALLister
		want    string
		wantErr bool
	}{
		{
			name: "no lister (tier2 not configured)",
			want: firstRequired,
		},
		{
			name:    "nothing pending",
			pending: fakePendingWALs{},
			want:    firstRequired,
		},
		{
			name:    "pending WAL older than the backups need is kept",
			pending: fakePendingWALs{oldest: "000000010000000000000004"},
			want:    "000000010000000000000004",
		},
		{
			name:    "pending WAL equal to the first required",
			pending: fakePendingWALs{oldest: firstRequired},
			want:    firstRequired,
		},
		{
			name:    "pending WAL newer than the backups need",
			pending: fakePendingWALs{oldest: "00000001000000000000000C"},
			want:    firstRequired,
		},
		{
			name:    "lookup failure removes nothing",
			pending: fakePendingWALs{err: errFakePendingWALs},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Sweeper{}
			got, err := s.holdForPendingWALs(t.Context(), "cluster-example",
				tierConfig{name: "tier1", pendingWALs: tt.pending}, firstRequired)
			if tt.wantErr {
				if !errors.Is(err, errFakePendingWALs) {
					t.Fatalf("holdForPendingWALs() error = %v, want %v", err, errFakePendingWALs)
				}

				return
			}
			if err != nil {
				t.Fatalf("holdForPendingWALs() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("holdForPendingWALs() = %q, want %q", got, tt.want)
			}
		})
	}
}

// stubBackupLister is a backupDeleter listing a fixed set of backups.
type stubBackupLister struct {
	backups klioclient.BackupList
}

func (s stubBackupLister) ListBackups(context.Context, string) (klioclient.BackupList, error) {
	return s.backups, nil
}

func (s stubBackupLister) DeleteBackup(context.Context, string, string) error {
	return nil
}

// walRetentionTestCluster is the cluster whose WALs the WAL retention tests
// prune.
const walRetentionTestCluster = "cluster-example"

// newTestWALRepository returns a WAL repository on an in-memory filesystem
// holding the given WAL segments of walRetentionTestCluster.
func newTestWALRepository(t *testing.T, walNames ...string) (*repository.Connection, afero.Fs) {
	t.Helper()

	opts := repository.Options{FS: afero.NewMemMapFs(), Password: "test-password"}
	if err := repository.Initialize(opts); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	conn, err := repository.Open(opts)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(conn.Close)

	for _, name := range walNames {
		if err := afero.WriteFile(opts.FS, walTestPath(name), []byte{}, 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error: %v", name, err)
		}
	}

	return conn, opts.FS
}

// walTestPath returns where the WAL repository stores walName of
// walRetentionTestCluster.
func walTestPath(walName string) string {
	return path.Join(walRetentionTestCluster, walName[:16], walName)
}

// remainingWALs returns which of walNames are still in fs.
func remainingWALs(t *testing.T, fs afero.Fs, walNames []string) []string {
	t.Helper()

	var remaining []string
	for _, name := range walNames {
		exists, err := afero.Exists(fs, walTestPath(name))
		if err != nil {
			t.Fatalf("Exists(%q) error: %v", name, err)
		}
		if exists {
			remaining = append(remaining, name)
		}
	}

	return remaining
}

// TestApplyWALRetentionKeepsWALsPendingRelay reproduces a tier2 outage on
// tier1: the only backup left starts at WAL 0A, but WALs 04 to 09 have not
// been relayed to tier2 yet, so they must stay on tier1.
func TestApplyWALRetentionKeepsWALsPendingRelay(t *testing.T) {
	walNames := []string{
		"000000010000000000000003",
		"000000010000000000000004",
		"000000010000000000000005",
		"000000010000000000000006",
		"000000010000000000000007",
		"000000010000000000000008",
		"000000010000000000000009",
		"00000001000000000000000A",
	}
	backups := klioclient.BackupList{{Name: "b2", StartWAL: "00000001000000000000000A"}}

	tests := []struct {
		name    string
		pending PendingWALLister
		want    []string
		wantErr bool
	}{
		{
			name: "no lister prunes up to the backups",
			want: walNames[7:],
		},
		{
			name:    "nothing pending prunes up to the backups",
			pending: fakePendingWALs{},
			want:    walNames[7:],
		},
		{
			name:    "pending WALs are kept",
			pending: fakePendingWALs{oldest: "000000010000000000000004"},
			want:    walNames[1:],
		},
		{
			name:    "lookup failure removes nothing",
			pending: fakePendingWALs{err: errFakePendingWALs},
			want:    walNames,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			walRepository, fs := newTestWALRepository(t, walNames...)
			s := &Sweeper{}

			err := s.applyWALRetention(t.Context(), walRetentionTestCluster, tierConfig{
				name:          "tier1",
				backupClient:  stubBackupLister{backups: backups},
				walRepository: walRepository,
				pendingWALs:   tt.pending,
			})
			if tt.wantErr != (err != nil) {
				t.Fatalf("applyWALRetention() error = %v, wantErr %t", err, tt.wantErr)
			}

			if got := remainingWALs(t, fs, walNames); !slices.Equal(got, tt.want) {
				t.Fatalf("remaining WALs = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTierConfigsWithoutTier2(t *testing.T) {
	configs := (&Sweeper{opts: &Options{PendingWALs: fakePendingWALs{}}}).tierConfigs()
	if len(configs) != 1 || configs[0].name != "tier1" {
		t.Fatalf("tierConfigs() = %v, want only tier1", configs)
	}
	if configs[0].pendingWALs != nil || configs[0].syncGuardClient != nil {
		t.Fatalf("without tier2, tier1 must not be guarded")
	}
}

func TestTierConfigsWithTier2(t *testing.T) {
	configs := (&Sweeper{
		opts:         &Options{PendingWALs: fakePendingWALs{}},
		tier2Enabled: true,
		tier2Client:  &klioclientkopia.Connection{},
	}).tierConfigs()
	if len(configs) != 2 || configs[0].name != "tier1" || configs[1].name != "tier2" {
		t.Fatalf("tierConfigs() = %v, want tier1 and tier2", configs)
	}
	if configs[0].pendingWALs == nil || configs[0].syncGuardClient == nil {
		t.Fatalf("tier1 must be guarded by its pending WALs and its tier2 backups")
	}
	if configs[1].pendingWALs != nil || configs[1].syncGuardClient != nil {
		t.Fatalf("tier2 has no further tier to be guarded against")
	}
}
