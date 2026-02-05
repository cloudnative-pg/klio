package queue

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startNATSServer starts an embedded NATS server for testing.
func startNATSServer(t *testing.T) (*server.Server, string) {
	t.Helper()

	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // random port
		JetStream: true,
	}

	ns, err := server.NewServer(opts)
	require.NoError(t, err, "failed to create NATS server")

	go ns.Start()

	if !ns.ReadyForConnections(4 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	return ns, ns.ClientURL()
}

func TestNew(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	conn, err := New(context.Background(), nc)
	require.NoError(t, err)
	assert.NotNil(t, conn)
}

func TestNotifyWALReceived(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// Test sending a notification
	task := &WALTask{
		ClusterName: "test-cluster",
		WALName:     "000000010000000000000001",
	}

	err = conn.NotifyWALReceived(ctx, task)
	require.NoError(t, err, "NotifyWALReceived should succeed")
}

func TestNotifyWALReceived_WithoutSetup(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// NATS JetStream will auto-create the stream, so this actually works
	// This test verifies that behavior
	task := &WALTask{
		ClusterName: "test-cluster",
		WALName:     "000000010000000000000001",
	}

	err = conn.NotifyWALReceived(ctx, task)
	require.NoError(t, err, "NotifyWALReceived should succeed even without explicit setup due to auto-create")
}

func TestWALTask_Serialization(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// Subscribe to the subject - use unique cluster name
	js, err := nc.JetStream()
	require.NoError(t, err)

	_, err = js.Subscribe("klio.serialization-cluster.wal", func(msg *nats.Msg) {
		assert.Contains(t, string(msg.Data), "serialization-cluster")
		assert.Contains(t, string(msg.Data), "000000010000000000000001")
	})
	require.NoError(t, err)

	// Send notification
	task := &WALTask{
		ClusterName: "serialization-cluster",
		WALName:     "000000010000000000000001",
	}

	err = conn.NotifyWALReceived(ctx, task)
	require.NoError(t, err)

	// Give it a moment to process
	time.Sleep(100 * time.Millisecond)
}

func TestGetOldestPendingWAL_EmptyQueue(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// No messages in queue - use unique cluster name to avoid interference
	oldestWAL, err := conn.GetOldestPendingWAL(ctx, "empty-queue-cluster")
	require.NoError(t, err)
	assert.Empty(t, oldestWAL, "should return empty string for empty queue")
}

func TestGetOldestPendingWAL_SingleMessage(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// Add one WAL to the queue - use unique cluster name
	task := &WALTask{
		ClusterName: "single-msg-cluster",
		WALName:     "000000010000000000000005",
	}
	err = conn.NotifyWALReceived(ctx, task)
	require.NoError(t, err)

	// Give NATS time to process
	time.Sleep(100 * time.Millisecond)

	oldestWAL, err := conn.GetOldestPendingWAL(ctx, "single-msg-cluster")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000005", oldestWAL)
}

func TestGetOldestPendingWAL_MultipleMessages(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// Add multiple WALs to the queue (out of order to test sorting)
	// Use unique cluster name
	wals := []string{
		"000000010000000000000005",
		"000000010000000000000003",
		"000000010000000000000007",
		"000000010000000000000001",
	}
	for _, wal := range wals {
		task := &WALTask{
			ClusterName: "multi-msg-cluster",
			WALName:     wal,
		}
		err = conn.NotifyWALReceived(ctx, task)
		require.NoError(t, err)
	}

	// Give NATS time to process
	time.Sleep(100 * time.Millisecond)

	oldestWAL, err := conn.GetOldestPendingWAL(ctx, "multi-msg-cluster")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000001", oldestWAL, "should return lexicographically smallest WAL")
}

func TestGetOldestPendingWAL_MultipleCluster(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// Add WALs for cluster-a
	clusterAWALs := []string{
		"000000010000000000000010",
		"000000010000000000000011",
	}
	for _, wal := range clusterAWALs {
		task := &WALTask{
			ClusterName: "cluster-a",
			WALName:     wal,
		}
		err = conn.NotifyWALReceived(ctx, task)
		require.NoError(t, err)
	}

	// Add WALs for cluster-b (older WALs)
	clusterBWALs := []string{
		"000000010000000000000001",
		"000000010000000000000002",
	}
	for _, wal := range clusterBWALs {
		task := &WALTask{
			ClusterName: "cluster-b",
			WALName:     wal,
		}
		err = conn.NotifyWALReceived(ctx, task)
		require.NoError(t, err)
	}

	// Give NATS time to process
	time.Sleep(100 * time.Millisecond)

	// Query cluster-a - should only see cluster-a WALs
	oldestWAL, err := conn.GetOldestPendingWAL(ctx, "cluster-a")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000010", oldestWAL, "should return oldest WAL for cluster-a only")

	// Query cluster-b - should only see cluster-b WALs
	oldestWAL, err = conn.GetOldestPendingWAL(ctx, "cluster-b")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000001", oldestWAL, "should return oldest WAL for cluster-b only")

	// Query non-existent cluster
	oldestWAL, err = conn.GetOldestPendingWAL(ctx, "cluster-c")
	require.NoError(t, err)
	assert.Empty(t, oldestWAL, "should return empty for non-existent cluster")
}

func TestGetOldestPendingWAL_DifferentTimelines(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// Add WALs with different timelines - use unique cluster name
	// Timeline 2 should be "older" than timeline 1 lexicographically
	wals := []string{
		"000000020000000000000001", // Timeline 2
		"000000010000000000000005", // Timeline 1
		"000000010000000000000001", // Timeline 1, oldest
	}
	for _, wal := range wals {
		task := &WALTask{
			ClusterName: "timeline-cluster",
			WALName:     wal,
		}
		err = conn.NotifyWALReceived(ctx, task)
		require.NoError(t, err)
	}

	// Give NATS time to process
	time.Sleep(100 * time.Millisecond)

	oldestWAL, err := conn.GetOldestPendingWAL(ctx, "timeline-cluster")
	require.NoError(t, err)
	// Lexicographically, "000000010000000000000001" < "000000010000000000000005" < "000000020000000000000001"
	assert.Equal(t, "000000010000000000000001", oldestWAL, "should return lexicographically smallest WAL")
}
