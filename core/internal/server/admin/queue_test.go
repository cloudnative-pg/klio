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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

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

// startQueueFixture starts NATS with the Klio streams and an admin server.
//
//nolint:ireturn // returns jetstream and gRPC client interfaces
func startQueueFixture(t *testing.T) (jetstream.JetStream, klioGRPC.AdminClient) {
	t.Helper()

	url := startNATSServer(t)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	_, err = queue.New(t.Context(), nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	return js, klioGRPC.NewAdminClient(startAdminServer(t, url))
}

// streamMsgs returns the message count of the named stream.
func streamMsgs(t *testing.T, js jetstream.JetStream, name string) uint64 {
	t.Helper()

	stream, err := js.Stream(t.Context(), name)
	require.NoError(t, err)

	info, err := stream.Info(t.Context())
	require.NoError(t, err)

	return info.State.Msgs
}

func TestQueueDiscardWALsOverSocket(t *testing.T) {
	seed := func(t *testing.T) (jetstream.JetStream, klioGRPC.AdminClient) {
		t.Helper()

		js, client := startQueueFixture(t)
		seedFailedTask(t, js, "klio.wal.cluster-a", walStreamName, walConsumerName,
			queue.WALTask{ClusterName: "cluster-a", WALName: "000000010000000000000001"}, 10)
		seedFailedTask(t, js, "klio.wal.cluster-a", walStreamName, walConsumerName,
			queue.WALTask{ClusterName: "cluster-a", WALName: "000000010000000000000002"}, 10)
		seedFailedTask(t, js, "klio.wal.cluster-b", walStreamName, walConsumerName,
			queue.WALTask{ClusterName: "cluster-b", WALName: "000000010000000000000003"}, 7)
		require.Equal(t, uint64(3), streamMsgs(t, js, walStreamName))

		return js, client
	}

	listWALs := func(t *testing.T, client klioGRPC.AdminClient) []string {
		t.Helper()

		resp, err := client.QueueListFailedWALs(t.Context(), &klioGRPC.QueueListFailedWALsRequest{})
		require.NoError(t, err)

		names := make([]string, 0, len(resp.GetWals()))
		for _, w := range resp.GetWals() {
			names = append(names, w.GetWalName())
		}

		return names
	}

	t.Run("by cluster and WAL name", func(t *testing.T) {
		js, client := seed(t)

		cluster := "cluster-a"
		resp, err := client.QueueDiscardWALs(t.Context(), &klioGRPC.QueueDiscardWALsRequest{
			ClusterName: &cluster,
			WalNames:    []string{"000000010000000000000001"},
		})
		require.NoError(t, err)
		assert.Equal(t, uint64(1), resp.GetDiscarded())

		assert.ElementsMatch(t,
			[]string{"000000010000000000000002", "000000010000000000000003"}, listWALs(t, client))
		assert.Equal(t, uint64(2), streamMsgs(t, js, walStreamName), "source message not released")
	})

	t.Run("by cluster", func(t *testing.T) {
		js, client := seed(t)

		cluster := "cluster-b"
		resp, err := client.QueueDiscardWALs(t.Context(),
			&klioGRPC.QueueDiscardWALsRequest{ClusterName: &cluster})
		require.NoError(t, err)
		assert.Equal(t, uint64(1), resp.GetDiscarded())

		assert.ElementsMatch(t,
			[]string{"000000010000000000000001", "000000010000000000000002"}, listWALs(t, client))
		assert.Equal(t, uint64(2), streamMsgs(t, js, walStreamName))
	})

	t.Run("every cluster", func(t *testing.T) {
		js, client := seed(t)

		resp, err := client.QueueDiscardWALs(t.Context(), &klioGRPC.QueueDiscardWALsRequest{})
		require.NoError(t, err)
		assert.Equal(t, uint64(3), resp.GetDiscarded())

		assert.Empty(t, listWALs(t, client))
		assert.Zero(t, streamMsgs(t, js, walStreamName))
	})

	t.Run("WAL names without a cluster are rejected", func(t *testing.T) {
		js, client := seed(t)

		_, err := client.QueueDiscardWALs(t.Context(), &klioGRPC.QueueDiscardWALsRequest{
			WalNames: []string{"000000010000000000000001"},
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		assert.Len(t, listWALs(t, client), 3)
		assert.Equal(t, uint64(3), streamMsgs(t, js, walStreamName))
	})
}

func TestQueueDiscardBackupsOverSocket(t *testing.T) {
	seed := func(t *testing.T) (jetstream.JetStream, klioGRPC.AdminClient) {
		t.Helper()

		js, client := startQueueFixture(t)
		seedFailedTask(t, js, "klio.backup.cluster-a", backupStreamName, backupConsumerName,
			queue.BackupTask{ClusterName: "cluster-a"}, 5)
		seedFailedTask(t, js, "klio.backup.cluster-b", backupStreamName, backupConsumerName,
			queue.BackupTask{ClusterName: "cluster-b"}, 3)
		require.Equal(t, uint64(2), streamMsgs(t, js, backupStreamName))

		return js, client
	}

	listClusters := func(t *testing.T, client klioGRPC.AdminClient) []string {
		t.Helper()

		resp, err := client.QueueListFailedBackups(t.Context(), &klioGRPC.QueueListFailedBackupsRequest{})
		require.NoError(t, err)

		clusters := make([]string, 0, len(resp.GetBackups()))
		for _, b := range resp.GetBackups() {
			clusters = append(clusters, b.GetClusterName())
		}

		return clusters
	}

	t.Run("by cluster", func(t *testing.T) {
		js, client := seed(t)

		cluster := "cluster-a"
		resp, err := client.QueueDiscardBackups(t.Context(),
			&klioGRPC.QueueDiscardBackupsRequest{ClusterName: &cluster})
		require.NoError(t, err)
		assert.Equal(t, uint64(1), resp.GetDiscarded())

		assert.Equal(t, []string{"cluster-b"}, listClusters(t, client))
		assert.Equal(t, uint64(1), streamMsgs(t, js, backupStreamName), "source message not released")
	})

	t.Run("every cluster", func(t *testing.T) {
		js, client := seed(t)

		resp, err := client.QueueDiscardBackups(t.Context(), &klioGRPC.QueueDiscardBackupsRequest{})
		require.NoError(t, err)
		assert.Equal(t, uint64(2), resp.GetDiscarded())

		assert.Empty(t, listClusters(t, client))
		assert.Zero(t, streamMsgs(t, js, backupStreamName))
	})
}

func TestQueueDiscardWithoutStreamManager(t *testing.T) {
	srv := &Server{}

	_, err := srv.QueueDiscardWALs(t.Context(), &klioGRPC.QueueDiscardWALsRequest{})
	assert.Equal(t, codes.Unavailable, status.Code(err))

	_, err = srv.QueueDiscardBackups(t.Context(), &klioGRPC.QueueDiscardBackupsRequest{})
	assert.Equal(t, codes.Unavailable, status.Code(err))
}
