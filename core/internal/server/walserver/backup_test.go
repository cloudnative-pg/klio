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

package walserver

import (
	"context"
	"path"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

const closeBackupSegmentSize = 16 * 1024 * 1024

// newTestImplementation returns a WAL server backed by an in-memory repository
// pre-populated with the given WAL files for a single cluster.
func newTestImplementation(t *testing.T, clusterName string, walFiles []string) *Implementation {
	t.Helper()

	opts := repository.Options{
		FS:       afero.NewMemMapFs(),
		Password: "test-password",
	}
	require.NoError(t, repository.Initialize(opts))

	conn, err := repository.Open(opts)
	require.NoError(t, err)
	t.Cleanup(conn.Close)

	for _, walName := range walFiles {
		walDir := path.Join(clusterName, walName[0:16])
		require.NoError(t, opts.FS.MkdirAll(walDir, 0o750))
		file, err := opts.FS.Create(path.Join(walDir, walName))
		require.NoError(t, err)
		require.NoError(t, file.Close())
	}

	return New(Options{Connection: conn})
}

// TestCloseBackupFailsOnPermanentlyMissingWAL verifies that CloseBackup returns
// a terminal error when a required WAL predates the earliest archived WAL and
// can therefore never be archived.
func TestCloseBackupFailsOnPermanentlyMissingWAL(t *testing.T) {
	const clusterName = "test-cluster"

	// The archive starts at segment 05: segments 03 and 04 required by the
	// backup will never appear.
	impl := newTestImplementation(t, clusterName, []string{
		"000000010000000000000005",
		"000000010000000000000006",
		"000000010000000000000007",
	})

	result, err := impl.CloseBackup(context.Background(), &grpc.CloseBackupRequest{
		ClusterName: clusterName,
		Timeline:    1,
		StartWal:    "000000010000000000000003",
		EndWal:      "000000010000000000000007",
		SegmentSize: closeBackupSegmentSize,
	})

	require.Error(t, err)
	require.Nil(t, result)

	s, ok := status.FromError(err)
	require.True(t, ok, "expected a gRPC status error")
	assert.Equal(t, codes.FailedPrecondition, s.Code())
}

// TestCloseBackupWaitsForRecentMissingWAL verifies that CloseBackup keeps
// reporting a not-yet-archived WAL as missing (so the client waits) when that
// WAL does not predate the earliest archived WAL.
func TestCloseBackupWaitsForRecentMissingWAL(t *testing.T) {
	const clusterName = "test-cluster"

	// Segment 06 is not archived yet, but it does not predate the earliest
	// archived WAL (03): it can still arrive.
	impl := newTestImplementation(t, clusterName, []string{
		"000000010000000000000003",
		"000000010000000000000004",
		"000000010000000000000005",
		"000000010000000000000007",
	})

	result, err := impl.CloseBackup(context.Background(), &grpc.CloseBackupRequest{
		ClusterName: clusterName,
		Timeline:    1,
		StartWal:    "000000010000000000000003",
		EndWal:      "000000010000000000000007",
		SegmentSize: closeBackupSegmentSize,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"000000010000000000000006"}, result.GetMissingWalFiles())
}

// TestCloseBackupWaitsForWALStillBeingStreamed verifies that the in-flight
// `.partial` file the WAL writer creates for the segment it is receiving does
// not make that same segment look permanently un-archivable. This is the state
// a freshly created cluster is in when its first backup closes.
func TestCloseBackupWaitsForWALStillBeingStreamed(t *testing.T) {
	const clusterName = "test-cluster"

	// Nothing is archived yet: segment 05 is still being received.
	impl := newTestImplementation(t, clusterName, []string{
		"000000010000000000000005.partial",
	})

	result, err := impl.CloseBackup(context.Background(), &grpc.CloseBackupRequest{
		ClusterName: clusterName,
		Timeline:    1,
		StartWal:    "000000010000000000000005",
		EndWal:      "000000010000000000000005",
		SegmentSize: closeBackupSegmentSize,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"000000010000000000000005"}, result.GetMissingWalFiles())
}
