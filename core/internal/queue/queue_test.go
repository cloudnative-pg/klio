package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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

func TestNotifyWALReceivedWithoutSetup(t *testing.T) {
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

func TestWALTaskSerialization(t *testing.T) {
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

func TestGetOldestPendingWALEmptyQueue(t *testing.T) {
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

func TestGetOldestPendingWALSingleMessage(t *testing.T) {
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

func TestGetOldestPendingWALMultipleMessages(t *testing.T) {
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

func TestGetOldestPendingWALMultipleCluster(t *testing.T) {
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

func TestGetOldestPendingWALDifferentTimelines(t *testing.T) {
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

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name        string
		walInfo     *jetstream.ConsumerInfo
		walErr      error
		backupInfo  *jetstream.ConsumerInfo
		backupErr   error
		expectedRes *Status
		expectErr   bool
	}{
		{
			name:        "Happy path",
			walInfo:     &jetstream.ConsumerInfo{NumPending: 10},
			backupInfo:  &jetstream.ConsumerInfo{NumPending: 5},
			expectedRes: &Status{PendingWALs: 10, PendingBackups: 5},
			expectErr:   false,
		},
		{
			name:      "WAL Info Error",
			walErr:    errors.New("connection failed"),
			expectErr: true,
		},
		{
			name:      "WAL Info Nil Return",
			walInfo:   nil,
			expectErr: true,
		},
		{
			name:      "Backup Info Error",
			walInfo:   &jetstream.ConsumerInfo{NumPending: 1},
			backupErr: errors.New("disk full"),
			expectErr: true,
		},
		{
			name:       "Backup Info Nil Return",
			backupInfo: nil,
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockWal := &MockConsumer{info: tt.walInfo, err: tt.walErr}
			mockBackup := &MockConsumer{info: tt.backupInfo, err: tt.backupErr}

			q := &Conn{
				walConsumer:    mockWal,
				backupConsumer: mockBackup,
			}

			status, err := q.GetStatus(context.Background())

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, status)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, status)
			assert.Equal(t, tt.expectedRes.PendingWALs, status.PendingWALs)
			assert.Equal(t, tt.expectedRes.PendingBackups, status.PendingBackups)
		})
	}
}

// MockConsumer uses embedding to satisfy the jetstream.Consumer interface
// without explicitly defining every method, bypassing ireturn issues.
type MockConsumer struct {
	jetstream.Consumer

	info *jetstream.ConsumerInfo
	err  error
}

func (m *MockConsumer) Info(_ context.Context) (*jetstream.ConsumerInfo, error) {
	return m.info, m.err
}

func (m *MockConsumer) CachedInfo() *jetstream.ConsumerInfo {
	return m.info
}
