package queue

import (
	"context"
	"encoding/json"
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
		StoreDir:  t.TempDir(),
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

func TestNewRemovesEmptyLegacyStream(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      legacyKlioStreamName,
		Retention: jetstream.WorkQueuePolicy,
		Subjects:  []string{"klio.*.wal", "klio.*.backup"},
		Storage:   jetstream.FileStorage,
	})
	require.NoError(t, err)

	_, err = New(ctx, nc)
	require.NoError(t, err)

	_, err = js.Stream(ctx, legacyKlioStreamName)
	require.ErrorIs(t, err, jetstream.ErrStreamNotFound,
		"empty legacy KLIO stream should be deleted on startup")
}

func TestNewMigratesNonEmptyLegacyStream(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      legacyKlioStreamName,
		Retention: jetstream.WorkQueuePolicy,
		Subjects:  []string{"klio.*.wal", "klio.*.backup"},
		Storage:   jetstream.FileStorage,
	})
	require.NoError(t, err)

	walPayload, err := json.Marshal(WALTask{
		ClusterName: "leftover",
		WALName:     "000000010000000000000001",
	})
	require.NoError(t, err)
	_, err = js.Publish(ctx, "klio.leftover.wal", walPayload)
	require.NoError(t, err)

	backupPayload, err := json.Marshal(BackupTask{ClusterName: "leftover"})
	require.NoError(t, err)
	_, err = js.Publish(ctx, "klio.leftover.backup", backupPayload)
	require.NoError(t, err)

	conn, err := New(ctx, nc)
	require.NoError(t, err)

	_, err = js.Stream(ctx, legacyKlioStreamName)
	require.ErrorIs(t, err, jetstream.ErrStreamNotFound,
		"legacy KLIO stream must be removed after migration")

	walMsg, err := conn.klioWalStream.GetLastMsgForSubject(ctx, walSubject("leftover"))
	require.NoError(t, err, "migrated WAL task must land on the new WAL stream")
	assert.JSONEq(t, string(walPayload), string(walMsg.Data))

	backupMsg, err := conn.klioBackupStream.GetLastMsgForSubject(ctx, backupSubject("leftover"))
	require.NoError(t, err, "migrated backup task must land on the new backup stream")
	assert.JSONEq(t, string(backupPayload), string(backupMsg.Data))
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

func TestWALTaskSerialization(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	task := &WALTask{
		ClusterName: "serialization-cluster",
		WALName:     "000000010000000000000001",
	}

	err = conn.NotifyWALReceived(ctx, task)
	require.NoError(t, err)

	// Read the message back from the WAL stream and verify its payload.
	msg, err := conn.klioWalStream.GetLastMsgForSubject(ctx, walSubject(task.ClusterName))
	require.NoError(t, err)
	assert.Contains(t, string(msg.Data), "serialization-cluster")
	assert.Contains(t, string(msg.Data), "000000010000000000000001")
}

// publishLatestUploadedWAL publishes directly to the latest-uploaded-WAL
// stream subject, simulating what the WAL consumer does after a successful
// tier2 upload.
func publishLatestUploadedWAL(t *testing.T, nc *nats.Conn, clusterName, walName string) {
	t.Helper()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	data, err := json.Marshal(WALTask{ClusterName: clusterName, WALName: walName})
	require.NoError(t, err)

	_, err = js.Publish(t.Context(), latestUploadedWalSubject(clusterName), data)
	require.NoError(t, err)
}

func TestNotifyBackupReceived(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	task := &BackupTask{
		ClusterName: "test-cluster",
	}

	err = conn.NotifyBackupReceived(ctx, task)
	require.NoError(t, err)
}

func TestBackupTaskSerialization(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	task := &BackupTask{
		ClusterName: "serialization-cluster",
	}

	err = conn.NotifyBackupReceived(ctx, task)
	require.NoError(t, err)

	msg, err := conn.klioBackupStream.GetLastMsgForSubject(ctx, backupSubject(task.ClusterName))
	require.NoError(t, err)
	assert.Contains(t, string(msg.Data), "serialization-cluster")
}

func TestStreamIsolation(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	require.NoError(t, conn.NotifyWALReceived(ctx, &WALTask{
		ClusterName: "iso-cluster",
		WALName:     "000000010000000000000001",
	}))
	require.NoError(t, conn.NotifyBackupReceived(ctx, &BackupTask{
		ClusterName: "iso-cluster",
	}))

	walInfo, err := conn.klioWalStream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), walInfo.State.Msgs, "WAL stream should have exactly 1 message")

	backupInfo, err := conn.klioBackupStream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), backupInfo.State.Msgs, "backup stream should have exactly 1 message")
}

func TestGetLatestUploadedWALEmpty(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	latestWAL, err := conn.GetLatestUploadedWAL(ctx, "empty-cluster")
	require.NoError(t, err)
	assert.Empty(t, latestWAL, "should return empty string when no WAL has been uploaded")
}

func TestGetLatestUploadedWALSingleMessage(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	publishLatestUploadedWAL(t, nc, "single-msg-cluster", "000000010000000000000005")

	latestWAL, err := conn.GetLatestUploadedWAL(ctx, "single-msg-cluster")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000005", latestWAL)
}

func TestGetLatestUploadedWALKeepsLatest(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	wals := []string{
		"000000010000000000000001",
		"000000010000000000000005",
		"000000010000000000000003",
		"000000010000000000000007",
	}
	for _, wal := range wals {
		publishLatestUploadedWAL(t, nc, "multi-msg-cluster", wal)
	}

	latestWAL, err := conn.GetLatestUploadedWAL(ctx, "multi-msg-cluster")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000007", latestWAL,
		"should return the most recently published WAL, regardless of WAL name ordering")
}

func TestGetLatestUploadedWALMultipleClusters(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	publishLatestUploadedWAL(t, nc, "cluster-a", "000000010000000000000010")
	publishLatestUploadedWAL(t, nc, "cluster-a", "000000010000000000000011")
	publishLatestUploadedWAL(t, nc, "cluster-b", "000000010000000000000001")
	publishLatestUploadedWAL(t, nc, "cluster-b", "000000010000000000000002")

	latestWAL, err := conn.GetLatestUploadedWAL(ctx, "cluster-a")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000011", latestWAL)

	latestWAL, err = conn.GetLatestUploadedWAL(ctx, "cluster-b")
	require.NoError(t, err)
	assert.Equal(t, "000000010000000000000002", latestWAL)

	latestWAL, err = conn.GetLatestUploadedWAL(ctx, "cluster-c")
	require.NoError(t, err)
	assert.Empty(t, latestWAL)
}

func TestGetStatus(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	status, err := conn.GetStatus(ctx)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, uint64(0), status.PendingWALs)
	assert.Equal(t, uint64(0), status.PendingBackups)

	require.NoError(t, conn.NotifyWALReceived(ctx, &WALTask{
		ClusterName: "status-cluster",
		WALName:     "000000010000000000000001",
	}))
	require.NoError(t, conn.NotifyWALReceived(ctx, &WALTask{
		ClusterName: "status-cluster",
		WALName:     "000000010000000000000002",
	}))
	require.NoError(t, conn.NotifyBackupReceived(ctx, &BackupTask{
		ClusterName: "status-cluster",
	}))

	require.Eventually(t, func() bool {
		status, err = conn.GetStatus(ctx)
		require.NoError(t, err)
		require.NotNil(t, status)

		return status.PendingWALs == 2 && status.PendingBackups == 1
	}, 2*time.Second, 50*time.Millisecond)
}
