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

package grpcclient

import (
	"context"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// newTestConnection creates a connection to a temporary, local Klio server
// backed by an in-memory Kopia repository, closing it automatically once the
// test finishes. This mirrors the setup already used by
// BenchmarkLookupSnapshotsViaKlioServer in connection_test.go.
func newTestConnection(ctx context.Context, t *testing.T, clusterName string) *Connection {
	t.Helper()

	conn, err := ConnectTemporary(
		ctx,
		log.GetLogger(),
		&config.ClientConfig{
			ClusterName: clusterName,
		},
		repository.Options{
			FS:       afero.NewMemMapFs(),
			Password: "random-string",
		},
	)
	require.NoError(t, err, "while creating temporary Klio repository")

	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	return &conn.Connection
}

func TestSendWALCoordinatorRequestStart(t *testing.T) {
	ctx := t.Context()

	t.Run("first contact, no prior metadata for the cluster", func(t *testing.T) {
		conn := newTestConnection(ctx, t, "cluster-name")
		coordinator := NewSendWALCoordinator(conn, false)

		// GIVEN a cluster the destination has never seen before
		// WHEN the source requests a start position
		walName, err := coordinator.RequestStart(ctx, "first-contact-cluster", "system-1", "000000010000000000000001")

		// THEN the destination has nothing to negotiate with, so it accepts
		// the source's own current WAL file name as the start position
		require.NoError(t, err)
		assert.Equal(t, "000000010000000000000001", walName)
	})

	t.Run("system ID mismatch on a second request for the same cluster", func(t *testing.T) {
		conn := newTestConnection(ctx, t, "cluster-name")
		coordinator := NewSendWALCoordinator(conn, false)

		// GIVEN a cluster that has already negotiated a start with system ID "system-a"
		_, err := coordinator.RequestStart(ctx, "mismatch-cluster", "system-a", "000000010000000000000001")
		require.NoError(t, err)

		// WHEN a different system ID negotiates a start for the same cluster name
		// THEN the destination rejects it, since it would mix WALs from two
		// different PostgreSQL instances under the same cluster name
		_, err = coordinator.RequestStart(ctx, "mismatch-cluster", "system-b", "000000010000000000000002")
		require.Error(t, err)
	})
}

func TestSendWALCoordinatorResetStream(t *testing.T) {
	ctx := t.Context()
	conn := newTestConnection(ctx, t, "reset-cluster")
	coordinator := NewSendWALCoordinator(conn, false)

	// GIVEN a cluster with an established start position and one archived WAL
	_, err := coordinator.RequestStart(ctx, "reset-cluster", "system-1", "000000010000000000000005")
	require.NoError(t, err)

	require.NoError(t, coordinator.StoreHistoryFile(ctx, "000000010000000000000005", []byte("wal-content")))

	t.Run("resetting past the latest archived WAL succeeds", func(t *testing.T) {
		// WHEN the source asks to reset the stream to a WAL file more recent
		// than what is archived
		walName, err := coordinator.ResetStream(ctx, "reset-cluster", "system-1", "000000010000000000000009")

		// THEN the destination accepts it and echoes back the requested WAL name
		require.NoError(t, err)
		assert.Equal(t, "000000010000000000000009", walName)
	})

	t.Run("resetting to or before the latest archived WAL fails", func(t *testing.T) {
		// WHEN the source asks to reset the stream to a WAL file that is not
		// more recent than what is already archived
		_, err := coordinator.ResetStream(ctx, "reset-cluster", "system-1", "000000010000000000000005")

		// THEN the destination refuses, since it would create a gap in the
		// archived WAL sequence
		require.Error(t, err)
	})
}

func TestSendWALCoordinatorStoreHistoryFile(t *testing.T) {
	ctx := t.Context()
	conn := newTestConnection(ctx, t, "history-cluster")

	t.Run("stores the content under the given name", func(t *testing.T) {
		// The temporary test server has no tier2 queue wired up, so this
		// exercises the tier1-only path; tier2 propagation is checked
		// structurally below, without a live send.
		coordinator := NewSendWALCoordinator(conn, false)

		err := coordinator.StoreHistoryFile(ctx, "00000001.history", []byte("history-file-content"))
		require.NoError(t, err)
	})

	t.Run("remembers the tier2 flag it was built with", func(t *testing.T) {
		coordinator := NewSendWALCoordinator(conn, true)
		assert.True(t, coordinator.sendToTier2)
	})
}
