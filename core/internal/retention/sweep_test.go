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
	"slices"
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// stubTier2Client is a minimal backupDeleter stub for testing
// protectUnsyncedToTier2 without a real Kopia client.
type stubTier2Client struct {
	backups klioclient.BackupList
	listErr error
}

func (s *stubTier2Client) ListBackups(_ context.Context, _ string) (klioclient.BackupList, error) {
	return s.backups, s.listErr
}

func (s *stubTier2Client) DeleteBackup(_ context.Context, _ string, _ string) error {
	return nil
}

// stubDeleter is a backupDeleter that records the deletions and fails the ones
// named in failing.
type stubDeleter struct {
	failing map[string]error
	deleted []string
}

func (s *stubDeleter) ListBackups(_ context.Context, _ string) (klioclient.BackupList, error) {
	return nil, nil
}

func (s *stubDeleter) DeleteBackup(_ context.Context, host string, name string) error {
	if err := s.failing[name]; err != nil {
		return err
	}

	s.deleted = append(s.deleted, host+"/"+name)

	return nil
}

func TestProtectUnsyncedToTier2(t *testing.T) {
	tests := []struct {
		name       string
		candidates klioclient.BackupList
		tier2      klioclient.BackupList
		tier2Err   error
		want       klioclient.BackupList
		wantErr    bool
	}{
		{
			name:       "candidate present on tier2 is kept for deletion",
			candidates: klioclient.BackupList{{Name: "backup-1"}},
			tier2:      klioclient.BackupList{{Name: "backup-1"}},
			want:       klioclient.BackupList{{Name: "backup-1"}},
		},
		{
			name:       "candidate absent from tier2 is protected (not deleted)",
			candidates: klioclient.BackupList{{Name: "backup-1"}},
			tier2:      klioclient.BackupList{},
			want:       nil,
		},
		{
			name:       "mixed candidates: only the synced one survives",
			candidates: klioclient.BackupList{{Name: "backup-1"}, {Name: "backup-2"}},
			tier2:      klioclient.BackupList{{Name: "backup-2"}},
			want:       klioclient.BackupList{{Name: "backup-2"}},
		},
		{
			name:       "tier2 listing error fails closed",
			candidates: klioclient.BackupList{{Name: "backup-1"}},
			tier2Err:   errors.New("boom"),
			want:       nil,
			wantErr:    true,
		},
		{
			name:       "no candidates is a no-op",
			candidates: nil,
			tier2:      klioclient.BackupList{{Name: "backup-1"}},
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubTier2Client{backups: tt.tier2, listErr: tt.tier2Err}

			got, err := protectUnsyncedToTier2(context.Background(), "cluster-a", tt.candidates, client)

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			sameName := func(a, b klioclient.BackupMetadata) bool { return a.Name == b.Name }
			if !slices.EqualFunc(got, tt.want, sameName) {
				t.Fatalf("protectUnsyncedToTier2() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPolicyFromProto(t *testing.T) {
	t.Run("a negative latest is rejected", func(t *testing.T) {
		_, err := policyFromProto(&grpc.TierRetentionPolicy{Latest: -1})
		if !errors.Is(err, ErrInvalidRetentionLatest) {
			t.Fatalf("policyFromProto() error = %v, want %v", err, ErrInvalidRetentionLatest)
		}
	})

	t.Run("zero and positive values are accepted", func(t *testing.T) {
		for _, latest := range []int32{0, 1, 10} {
			if _, err := policyFromProto(&grpc.TierRetentionPolicy{Latest: latest}); err != nil {
				t.Errorf("policyFromProto(latest=%d) unexpected error: %v", latest, err)
			}
		}
	})
}

func TestDeleteBackupsContinuesAfterAFailure(t *testing.T) {
	errBoom := errors.New("boom")
	deleter := &stubDeleter{failing: map[string]error{"b": errBoom}}

	deleted, err := deleteBackups(context.Background(), deleter, []hostBackupName{
		{Host: "cluster", Name: "a"},
		{Host: "cluster", Name: "b"},
		{Host: "cluster", Name: "c"},
	})

	if deleted != 2 {
		t.Errorf("deleteBackups() deleted %d backups, want 2", deleted)
	}

	if !errors.Is(err, errBoom) {
		t.Errorf("deleteBackups() error = %v, want it to wrap %v", err, errBoom)
	}

	if want := []string{"cluster/a", "cluster/c"}; !slices.Equal(deleter.deleted, want) {
		t.Errorf("deleted backups = %v, want %v", deleter.deleted, want)
	}
}

func TestDeleteBackupsWithoutFailures(t *testing.T) {
	deleter := &stubDeleter{}

	deleted, err := deleteBackups(context.Background(), deleter, []hostBackupName{
		{Host: "cluster", Name: "a"},
	})

	if err != nil || deleted != 1 {
		t.Errorf("deleteBackups() = (%d, %v), want (1, nil)", deleted, err)
	}
}

func TestBackupNames(t *testing.T) {
	got := backupNames([]hostBackupName{{Host: "h1", Name: "a"}, {Host: "h2", Name: "b"}})

	if want := []string{"h1/a", "h2/b"}; !slices.Equal(got, want) {
		t.Errorf("backupNames() = %v, want %v", got, want)
	}
}
