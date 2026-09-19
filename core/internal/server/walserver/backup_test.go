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
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

const (
	testClusterName = "cluster-example"
	testSegmentSize = 16 * 1024 * 1024
	testStartWAL    = "000000010000000000000001"
	testEndWAL      = "000000010000000000000002"
)

// newCloseBackupServer builds a WAL server whose repository holds only the
// backup's first WAL segment, so the last one is always reported missing,
// and whose queue is backed by an embedded NATS server. It returns the
// server and the queue connection to consume the enqueued tasks from.
func newCloseBackupServer(t *testing.T) (*Implementation, *queue.Conn) {
	t.Helper()

	ns, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	require.NoError(t, err)
	go ns.Start()
	require.True(t, ns.ReadyForConnections(4*time.Second), "NATS server not ready")
	t.Cleanup(ns.Shutdown)

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	q, err := queue.New(context.Background(), nc)
	require.NoError(t, err)

	fs := afero.NewMemMapFs()
	repoOpts := repository.Options{FS: fs, Password: "test-password"}
	require.NoError(t, repository.Initialize(repoOpts))
	conn, err := repository.Open(repoOpts)
	require.NoError(t, err)
	t.Cleanup(conn.Close)

	// Only the first segment has been archived.
	walPath := path.Join(testClusterName, testStartWAL[:16], testStartWAL)
	require.NoError(t, afero.WriteFile(fs, walPath, []byte("wal"), 0o600))

	return New(Options{Connection: conn, Queue: q}), q
}

// receiveBackupTask consumes one backup task from the queue, or returns nil
// when none arrives within the timeout.
func receiveBackupTask(t *testing.T, q *queue.Conn, timeout time.Duration) *queue.BackupTask {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	received := make(chan *queue.BackupTask, 1)
	go func() {
		_ = q.ConsumeBackupReceivedMessages(ctx, func(_ context.Context, task *queue.BackupTask) error {
			received <- task
			cancel()

			return nil
		})
	}()

	select {
	case task := <-received:
		return task
	case <-ctx.Done():
		return nil
	}
}

func newCloseBackupRequest(enqueueWithoutWALs bool) *grpc.CloseBackupRequest {
	return &grpc.CloseBackupRequest{
		ClusterName:        testClusterName,
		BackupName:         "backup-1",
		Timeline:           1,
		StartWal:           testStartWAL,
		EndWal:             testEndWAL,
		SegmentSize:        testSegmentSize,
		SendToTier2:        true,
		EnqueueWithoutWals: enqueueWithoutWALs,
	}
}

// TestCloseBackupMissingWALsWithoutWaitEnqueuesTask covers a backup taken
// with a client that does not wait for the last WAL to be archived, so the post-backup
// task must be enqueued on this single call or the backup is never relayed.
func TestCloseBackupMissingWALsWithoutWaitEnqueuesTask(t *testing.T) {
	impl, q := newCloseBackupServer(t)

	result, err := impl.CloseBackup(context.Background(), newCloseBackupRequest(true))
	require.NoError(t, err)
	require.Equal(t, []string{testEndWAL}, result.GetMissingWalFiles())
	require.True(t, result.GetTier2Schedule())

	task := receiveBackupTask(t, q, 5*time.Second)
	require.NotNil(t, task, "the backup task must be enqueued even if WALs are missing")
	require.Equal(t, testClusterName, task.ClusterName)
	require.True(t, task.SendToTier2)
}

// TestCloseBackupMissingWALsWithWaitDefersTask covers a backup taken with a client
// that waits for the last WAL to be archived: the client retries CloseBackup until no WAL is missing, so the
// task must not be enqueued before then.
func TestCloseBackupMissingWALsWithWaitDefersTask(t *testing.T) {
	impl, q := newCloseBackupServer(t)

	result, err := impl.CloseBackup(context.Background(), newCloseBackupRequest(false))
	require.NoError(t, err)
	require.Equal(t, []string{testEndWAL}, result.GetMissingWalFiles())
	require.False(t, result.GetTier2Schedule())

	require.Nil(t, receiveBackupTask(t, q, time.Second),
		"no backup task must be enqueued while the client is still waiting for WALs")
}
