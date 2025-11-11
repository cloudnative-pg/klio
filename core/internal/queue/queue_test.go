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

	conn := New(nc)
	assert.NotNil(t, conn)
}

func TestEnsureSetup(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	conn := New(nc)
	ctx := context.Background()

	err = conn.EnsureSetup(ctx)
	require.NoError(t, err, "EnsureSetup should succeed")

	// Call again to test idempotency
	err = conn.EnsureSetup(ctx)
	require.NoError(t, err, "EnsureSetup should be idempotent")
}

func TestNotifyWALReceived(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	conn := New(nc)
	ctx := context.Background()

	// Setup the stream first
	err = conn.EnsureSetup(ctx)
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

	conn := New(nc)
	ctx := context.Background()

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

	conn := New(nc)
	ctx := context.Background()

	err = conn.EnsureSetup(ctx)
	require.NoError(t, err)

	// Subscribe to the subject
	js, err := nc.JetStream()
	require.NoError(t, err)

	_, err = js.Subscribe("klio.test-cluster.wal", func(msg *nats.Msg) {
		assert.Contains(t, string(msg.Data), "test-cluster")
		assert.Contains(t, string(msg.Data), "000000010000000000000001")
	})
	require.NoError(t, err)

	// Send notification
	task := &WALTask{
		ClusterName: "test-cluster",
		WALName:     "000000010000000000000001",
	}

	err = conn.NotifyWALReceived(ctx, task)
	require.NoError(t, err)

	// Give it a moment to process
	time.Sleep(100 * time.Millisecond)
}
