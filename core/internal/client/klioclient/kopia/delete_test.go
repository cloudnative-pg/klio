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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

var errDeleteFailed = errors.New("no snapshots matched")

// fakeSnapshotStore serves a scripted sequence of snapshot listings and records
// the IDs it was asked to delete. Listings are consumed one per call so a test
// can model the manifest IDs changing between attempts.
type fakeSnapshotStore struct {
	listings  [][]kopia.Manifest
	listCalls int

	// failDeleteOf holds the IDs whose deletion fails.
	failDeleteOf map[string]bool

	deleted []string
	listErr error
}

func (f *fakeSnapshotStore) ListSnapshots(
	_ context.Context,
	_ map[string]string,
	_ kopia.LogFunc,
) ([]kopia.Manifest, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	f.listCalls++

	// The last scripted listing is reused for any further attempt.
	idx := min(f.listCalls-1, len(f.listings)-1)

	return f.listings[idx], nil
}

func (f *fakeSnapshotStore) DeleteSnapshot(_ context.Context, id string) error {
	if f.failDeleteOf[id] {
		return errDeleteFailed
	}

	f.deleted = append(f.deleted, id)

	return nil
}

func manifest(id, host string) kopia.Manifest {
	return kopia.Manifest{ID: id, Source: kopia.SourceInfo{Host: host}}
}

func TestDeleteBackupSnapshots(t *testing.T) {
	ctx := context.Background()

	oldDelay := deleteBackupRetryDelay
	deleteBackupRetryDelay = 0
	t.Cleanup(func() { deleteBackupRetryDelay = oldDelay })

	t.Run("deletes every snapshot of the backup on the host", func(t *testing.T) {
		store := &fakeSnapshotStore{
			listings: [][]kopia.Manifest{
				{manifest("a", "cluster"), manifest("b", "cluster")},
				{},
			},
		}

		require.NoError(t, deleteBackupSnapshots(ctx, store, "cluster", "backup-1"))
		assert.Equal(t, []string{"a", "b"}, store.deleted)
		assert.Equal(t, 1, store.listCalls)
	})

	t.Run("ignores snapshots belonging to another host", func(t *testing.T) {
		store := &fakeSnapshotStore{
			listings: [][]kopia.Manifest{{manifest("a", "other-cluster")}},
		}

		err := deleteBackupSnapshots(ctx, store, "cluster", "backup-1")

		require.ErrorIs(t, err, ErrBackupNotFound)
		assert.Empty(t, store.deleted)
	})

	t.Run("returns not found when the backup has no snapshots", func(t *testing.T) {
		store := &fakeSnapshotStore{listings: [][]kopia.Manifest{{}}}

		require.ErrorIs(t, deleteBackupSnapshots(ctx, store, "cluster", "backup-1"), ErrBackupNotFound)
	})

	// A concurrent "kopia snapshot pin" rewrites a snapshot's manifest under a
	// new ID, so deleting the ID we listed matches nothing. Re-resolving must
	// pick up the new ID and finish the deletion.
	t.Run("retries with the rewritten manifest ID", func(t *testing.T) {
		store := &fakeSnapshotStore{
			listings: [][]kopia.Manifest{
				{manifest("a", "cluster"), manifest("stale", "cluster")},
				{manifest("rewritten", "cluster")},
				{},
			},
			failDeleteOf: map[string]bool{"stale": true},
		}

		require.NoError(t, deleteBackupSnapshots(ctx, store, "cluster", "backup-1"))
		assert.Equal(t, []string{"a", "rewritten"}, store.deleted)
		assert.Equal(t, 2, store.listCalls)
	})

	t.Run("gives up after the attempt budget and reports the failure", func(t *testing.T) {
		store := &fakeSnapshotStore{
			listings:     [][]kopia.Manifest{{manifest("stuck", "cluster")}},
			failDeleteOf: map[string]bool{"stuck": true},
		}

		err := deleteBackupSnapshots(ctx, store, "cluster", "backup-1")

		require.ErrorIs(t, err, errDeleteFailed)
		assert.Equal(t, deleteBackupAttempts, store.listCalls)
		assert.Empty(t, store.deleted)
	})

	t.Run("a listing failure is returned as is", func(t *testing.T) {
		listErr := errors.New("connection refused")
		store := &fakeSnapshotStore{listErr: listErr}

		require.ErrorIs(t, deleteBackupSnapshots(ctx, store, "cluster", "backup-1"), listErr)
	})
}
