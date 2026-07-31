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

package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// The following mirror the stream and consumer names owned by the queue
// package. They are duplicated here intentionally: the test seeds the DLQ
// by publishing onto the max-deliveries advisory subject, which is the same
// wire contract the running server relies on.
const (
	walStreamName      = "klio-wal-stream"
	walConsumerName    = "klio-wal-consumer"
	backupStreamName   = "klio-backup-stream"
	backupConsumerName = "klio-backup-consumer"
)

// startNATSServer starts an embedded NATS server with JetStream enabled.
func startNATSServer(t *testing.T) string {
	t.Helper()

	ns, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	require.NoError(t, err, "failed to create NATS server")

	go ns.Start()
	t.Cleanup(ns.Shutdown)

	if !ns.ReadyForConnections(4 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	return ns.ClientURL()
}

// seedFailedTask publishes a task onto its source subject and then writes a
// synthetic max-deliveries advisory pointing at it, mimicking the DLQ entry
// the server produces once a task exhausts its retries.
func seedFailedTask(
	t *testing.T,
	js jetstream.JetStream,
	subject, sourceStream, sourceConsumer string,
	task any,
	deliveries uint64,
) {
	t.Helper()

	payload, err := json.Marshal(task)
	require.NoError(t, err)

	ack, err := js.Publish(t.Context(), subject, payload)
	require.NoError(t, err)

	advisory, err := json.Marshal(server.JSConsumerDeliveryExceededAdvisory{
		Stream:     sourceStream,
		Consumer:   sourceConsumer,
		StreamSeq:  ack.Sequence,
		Deliveries: deliveries,
	})
	require.NoError(t, err)

	advisorySubject := fmt.Sprintf("%s.%s.%s",
		server.JSAdvisoryConsumerMaxDeliveryExceedPre, sourceStream, sourceConsumer)

	_, err = js.Publish(t.Context(), advisorySubject, advisory)
	require.NoError(t, err)
}

// startAdminServer brings up the admin gRPC server on a Unix socket backed by
// the given NATS URL and returns a connection dialed over that socket.
func startAdminServer(t *testing.T, queueURL string) *grpc.ClientConn {
	t.Helper()

	// Keep the socket under /tmp: the full t.TempDir() path exceeds the
	// ~104-byte sun_path limit for Unix sockets on macOS.
	//nolint:usetesting // t.TempDir() paths are too long for a Unix socket.
	socketDir, err := os.MkdirTemp("/tmp", "klio-admin")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })

	socketPath := filepath.Join(socketDir, "admin.sock")
	srv := &Server{opts: Options{QueueURL: queueURL, SocketPath: socketPath}}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		assert.NoError(t, srv.Start(ctx))
	}()

	require.Eventually(t, func() bool {
		_, err := os.Stat(socketPath)

		return err == nil
	}, 4*time.Second, 20*time.Millisecond, "admin socket was never created")

	conn, err := grpc.NewClient(
		"unix:"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func TestQueueListFailedWALsOverSocket(t *testing.T) {
	url := startNATSServer(t)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	// Provision the Klio streams (including the DLQ stream) so the seeded
	// advisories are captured.
	_, err = queue.New(t.Context(), nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedTask(t, js, "klio.wal.cluster-a", walStreamName, walConsumerName,
		queue.WALTask{ClusterName: "cluster-a", WALName: "000000010000000000000001"}, 10)
	seedFailedTask(t, js, "klio.wal.cluster-a", walStreamName, walConsumerName,
		queue.WALTask{ClusterName: "cluster-a", WALName: "000000010000000000000002"}, 10)
	seedFailedTask(t, js, "klio.wal.cluster-b", walStreamName, walConsumerName,
		queue.WALTask{ClusterName: "cluster-b", WALName: "000000010000000000000003"}, 7)

	client := klioGRPC.NewAdminClient(startAdminServer(t, url))

	t.Run("filtered by cluster", func(t *testing.T) {
		cluster := "cluster-a"
		resp, err := client.QueueListFailedWALs(t.Context(),
			&klioGRPC.QueueListFailedWALsRequest{ClusterName: &cluster})
		require.NoError(t, err)
		require.Len(t, resp.GetWals(), 2)

		names := make([]string, 0, len(resp.GetWals()))
		for _, w := range resp.GetWals() {
			assert.Equal(t, "cluster-a", w.GetClusterName())
			assert.NotZero(t, w.GetSequence(), "DLQ sequence must be populated")
			assert.NotNil(t, w.GetLastAttemptTime())
			names = append(names, w.GetWalName())
		}
		assert.ElementsMatch(t,
			[]string{"000000010000000000000001", "000000010000000000000002"}, names)
	})

	t.Run("unfiltered returns every cluster", func(t *testing.T) {
		resp, err := client.QueueListFailedWALs(t.Context(), &klioGRPC.QueueListFailedWALsRequest{})
		require.NoError(t, err)
		assert.Len(t, resp.GetWals(), 3)
	})
}

func TestQueueListFailedBackupsOverSocket(t *testing.T) {
	url := startNATSServer(t)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	_, err = queue.New(t.Context(), nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedTask(t, js, "klio.backup.cluster-a", backupStreamName, backupConsumerName,
		queue.BackupTask{ClusterName: "cluster-a"}, 5)
	seedFailedTask(t, js, "klio.backup.cluster-b", backupStreamName, backupConsumerName,
		queue.BackupTask{ClusterName: "cluster-b"}, 3)

	client := klioGRPC.NewAdminClient(startAdminServer(t, url))

	t.Run("filtered by cluster", func(t *testing.T) {
		cluster := "cluster-a"
		resp, err := client.QueueListFailedBackups(t.Context(),
			&klioGRPC.QueueListFailedBackupsRequest{ClusterName: &cluster})
		require.NoError(t, err)
		require.Len(t, resp.GetBackups(), 1)

		backup := resp.GetBackups()[0]
		assert.Equal(t, "cluster-a", backup.GetClusterName())
		assert.NotNil(t, backup.GetLastAttemptTime())
	})

	t.Run("unfiltered returns every cluster", func(t *testing.T) {
		resp, err := client.QueueListFailedBackups(t.Context(), &klioGRPC.QueueListFailedBackupsRequest{})
		require.NoError(t, err)
		assert.Len(t, resp.GetBackups(), 2)
	})
}
