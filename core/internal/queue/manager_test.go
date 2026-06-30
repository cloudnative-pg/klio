package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/jsm.go"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManager spins up an embedded NATS server, provisions the Klio
// streams via New(), and returns a StreamManager wired to the same
// connection so DLQ-based queries can be exercised end-to-end.
func newTestManager(t *testing.T) (*nats.Conn, *Conn, *StreamManager) {
	t.Helper()

	ns, url := startNATSServer(t)
	t.Cleanup(ns.Shutdown)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	conn, err := New(context.Background(), nc)
	require.NoError(t, err)

	mgr, err := NewStreamManager(nc)
	require.NoError(t, err)

	return nc, conn, mgr
}

// newBareManager spins up an embedded NATS server and a StreamManager
// WITHOUT provisioning any stream (New() is not called), so queries can be
// exercised against a server where the Klio streams do not exist yet.
func newBareManager(t *testing.T) *StreamManager {
	t.Helper()

	ns, url := startNATSServer(t)
	t.Cleanup(ns.Shutdown)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	mgr, err := NewStreamManager(nc)
	require.NoError(t, err)

	return mgr
}

// publishDLQAdvisory writes a synthetic max-deliveries advisory that
// points at the given source-stream sequence onto the matching DLQ
// stream subject for the given source stream + consumer pair.
func publishDLQAdvisory(t *testing.T, nc *nats.Conn, sourceStream, sourceConsumer string, streamSeq uint64) {
	t.Helper()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	subject := fmt.Sprintf("%s.%s.%s",
		server.JSAdvisoryConsumerMaxDeliveryExceedPre,
		sourceStream,
		sourceConsumer)

	payload, err := json.Marshal(server.JSConsumerDeliveryExceededAdvisory{
		Stream:    sourceStream,
		Consumer:  sourceConsumer,
		StreamSeq: streamSeq,
	})
	require.NoError(t, err)

	_, err = js.Publish(t.Context(), subject, payload)
	require.NoError(t, err)
}

// publishWALAndCaptureSeq publishes a WAL task and returns the stream
// sequence assigned by the WAL stream, so tests can build advisories
// that point at it.
func publishWALAndCaptureSeq(t *testing.T, conn *Conn, task *WALTask) uint64 {
	t.Helper()

	ctx := t.Context()
	require.NoError(t, conn.NotifyWALReceived(ctx, task))

	msg, err := streamHandle(ctx, t, conn.conn, klioWalStreamName).GetLastMsgForSubject(ctx, walSubject(task.ClusterName))
	require.NoError(t, err)

	return msg.Sequence
}

// publishBackupAndCaptureSeq publishes a backup task and returns the
// stream sequence assigned by the backup stream.
func publishBackupAndCaptureSeq(t *testing.T, conn *Conn, task *BackupTask) uint64 {
	t.Helper()

	ctx := t.Context()
	require.NoError(t, conn.NotifyBackupReceived(ctx, task))

	backupStream := streamHandle(ctx, t, conn.conn, klioBackupStreamName)
	msg, err := backupStream.GetLastMsgForSubject(ctx, backupSubject(task.ClusterName))
	require.NoError(t, err)

	return msg.Sequence
}

func TestGetStatus(t *testing.T) {
	_, conn, mgr := newTestManager(t)

	s, err := mgr.GetStatus()
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, uint64(0), s.PendingWALs)
	assert.Equal(t, uint64(0), s.PendingBackups)

	require.NoError(t, conn.NotifyWALReceived(t.Context(), &WALTask{
		ClusterName: "status-cluster",
		WALName:     "000000010000000000000001",
	}))
	require.NoError(t, conn.NotifyWALReceived(t.Context(), &WALTask{
		ClusterName: "status-cluster",
		WALName:     "000000010000000000000002",
	}))
	require.NoError(t, conn.NotifyBackupReceived(t.Context(), &BackupTask{
		ClusterName: "status-cluster",
	}))

	require.Eventually(t, func() bool {
		s, err = mgr.GetStatus()
		require.NoError(t, err)
		require.NotNil(t, s)

		return s.PendingWALs == 2 && s.PendingBackups == 1
	}, 2*time.Second, 50*time.Millisecond)
}

func TestGetStatusMissingStreams(t *testing.T) {
	mgr := newBareManager(t)

	s, err := mgr.GetStatus()
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, uint64(0), s.PendingWALs)
	assert.Equal(t, uint64(0), s.PendingBackups)
}

func TestListFailedWALTasksMissingStreams(t *testing.T) {
	mgr := newBareManager(t)

	tasks, err := mgr.ListFailedWALTasks(t.Context())
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestListFailedBackupTasksMissingStreams(t *testing.T) {
	mgr := newBareManager(t)

	tasks, err := mgr.ListFailedBackupTasks(t.Context())
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestListFailedWALTasksEmpty(t *testing.T) {
	_, _, mgr := newTestManager(t)

	tasks, err := mgr.ListFailedWALTasks(t.Context())
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

// seedSingleWALDLQEntry publishes one WAL task plus its DLQ advisory and
// returns the loaded DLQ and source streams once the advisory is visible.
func seedSingleWALDLQEntry(t *testing.T, nc *nats.Conn, conn *Conn, mgr *StreamManager) (*jsm.Stream, *jsm.Stream) {
	t.Helper()

	seq := publishWALAndCaptureSeq(t, conn, &WALTask{
		ClusterName: "short-read-cluster",
		WALName:     "000000010000000000000001",
	})
	publishDLQAdvisory(t, nc, klioWalStreamName, klioWalConsumerName, seq)

	dlq, err := mgr.mgr.LoadStream(klioDLQWalStreamName)
	require.NoError(t, err)
	source, err := mgr.mgr.LoadStream(klioWalStreamName)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		state, stateErr := dlq.State()
		return stateErr == nil && state.Msgs == 1
	}, 2*time.Second, 50*time.Millisecond)

	return dlq, source
}

// TestListPagerShortRead asserts that when the pager delivers fewer entries
// than the stream reported (e.g. a timeout against a slow/unreachable server),
// listPager fails loudly rather than returning a truncated listing.
func TestListPagerShortRead(t *testing.T) {
	nc, conn, mgr := newTestManager(t)

	dlq, source := seedSingleWALDLQEntry(t, nc, conn, mgr)

	pgr, err := dlq.PageContents()
	require.NoError(t, err)
	defer func() { _ = pgr.Close() }()

	// Claim the stream holds two entries while the pager will only ever deliver
	// the single seeded one, simulating an early pager termination.
	_, err = listPager[WALTask](t.Context(), pgr, source.ReadMessage, 2, listConfig{})
	require.ErrorIs(t, err, errIncompleteDLQListing)
}

// TestListPagerContextCancelled asserts that a cancelled caller context is
// surfaced as an error instead of being mistaken for end-of-stream.
func TestListPagerContextCancelled(t *testing.T) {
	nc, conn, mgr := newTestManager(t)

	dlq, source := seedSingleWALDLQEntry(t, nc, conn, mgr)

	pgr, err := dlq.PageContents()
	require.NoError(t, err)
	defer func() { _ = pgr.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = listPager[WALTask](ctx, pgr, source.ReadMessage, 1, listConfig{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestListFailedWALTasks(t *testing.T) {
	nc, conn, mgr := newTestManager(t)

	startedAt := time.Now()

	tasks := []WALTask{
		{ClusterName: "cluster-a", WALName: "000000010000000000000001"},
		{ClusterName: "cluster-a", WALName: "000000010000000000000002"},
		{ClusterName: "cluster-c", WALName: "000000010000000000000003"},
	}
	for i := range tasks {
		seq := publishWALAndCaptureSeq(t, conn, &tasks[i])
		publishDLQAdvisory(t, nc, klioWalStreamName, klioWalConsumerName, seq)
	}

	want := []WALTask{
		{ClusterName: "cluster-a", WALName: "000000010000000000000001"},
		{ClusterName: "cluster-a", WALName: "000000010000000000000002"},
	}

	got, err := mgr.ListFailedWALTasks(t.Context(), WithCluster("cluster-a"))
	require.NoError(t, err)
	require.Len(t, got, len(want))

	gotTasks := make([]WALTask, 0, len(got))
	for _, ft := range got {
		assert.NotZero(t, ft.Sequence, "DLQ entry must carry the DLQ stream sequence")

		assert.WithinRange(t, ft.Timestamp, startedAt, time.Now().Add(time.Second),
			"DLQ entry timestamp must reflect when the advisory was stored")

		gotTasks = append(gotTasks, ft.Task)
	}
	assert.ElementsMatch(t, want, gotTasks,
		"every failed task payload must be recoverable from FailedTask.Task")
}

func TestListFailedWALTasksMissingSourceMessage(t *testing.T) {
	nc, _, mgr := newTestManager(t)

	// Advisory references a sequence that has never been published, mimicking
	// an orphaned DLQ entry whose source message was retried or purged. The
	// orphaned entry must be skipped rather than failing the whole listing.
	publishDLQAdvisory(t, nc, klioWalStreamName, klioWalConsumerName, 9999)

	tasks, err := mgr.ListFailedWALTasks(t.Context())
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestListFailedBackupTasksEmpty(t *testing.T) {
	_, _, mgr := newTestManager(t)

	tasks, err := mgr.ListFailedBackupTasks(t.Context())
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestListFailedBackupTasks(t *testing.T) {
	nc, conn, mgr := newTestManager(t)

	startedAt := time.Now()

	want := []BackupTask{
		{ClusterName: "cluster-a"},
		{ClusterName: "cluster-b"},
	}
	for i := range want {
		seq := publishBackupAndCaptureSeq(t, conn, &want[i])
		publishDLQAdvisory(t, nc, klioBackupStreamName, klioBackupConsumerName, seq)
	}

	got, err := mgr.ListFailedBackupTasks(t.Context())
	require.NoError(t, err)
	require.Len(t, got, len(want))

	gotTasks := make([]BackupTask, 0, len(got))
	for _, ft := range got {
		assert.NotZero(t, ft.Sequence)
		assert.WithinRange(t, ft.Timestamp, startedAt, time.Now().Add(time.Second))

		gotTasks = append(gotTasks, ft.Task)
	}
	assert.ElementsMatch(t, want, gotTasks)
}

func TestListFailedBackupTasksMissingSourceMessage(t *testing.T) {
	nc, _, mgr := newTestManager(t)

	// An orphaned DLQ entry (source message retried or purged) must be skipped
	// rather than failing the whole listing.
	publishDLQAdvisory(t, nc, klioBackupStreamName, klioBackupConsumerName, 9999)

	tasks, err := mgr.ListFailedBackupTasks(t.Context())
	require.NoError(t, err)
	assert.Empty(t, tasks)
}
