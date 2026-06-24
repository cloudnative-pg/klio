package queue

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
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

// TestConsumerRetryConfig pins the bounded-retry configuration of the
// production WAL and backup consumers so that accidental regressions
// (e.g. dropping MaxDeliver or BackOff) are caught.
func TestConsumerRetryConfig(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	walInfo, err := conn.walConsumer.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10, walInfo.Config.MaxDeliver, "WAL consumer must bound retries")
	assert.Equal(t, []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		5 * time.Minute,
	}, walInfo.Config.BackOff)

	backupInfo, err := conn.backupConsumer.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, backupInfo.Config.MaxDeliver, "backup consumer must bound retries")
	assert.Equal(t, []time.Duration{
		5 * time.Minute,
		10 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
	}, backupInfo.Config.BackOff)
}

// newTestConsumer creates a dedicated work-queue stream and a durable consumer
// with a short AckWait and a small MaxDeliver, so retry/termination behavior
// can be exercised quickly without the long production backoff schedule.
//
//nolint:ireturn // mirrors the jetstream API surface
func newTestConsumer(
	t *testing.T,
	nc *nats.Conn,
	subject string,
	maxDeliver int,
	ackWait time.Duration,
) (jetstream.JetStream, jetstream.Consumer) {
	t.Helper()

	ctx := context.Background()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "test-stream",
		Retention: jetstream.WorkQueuePolicy,
		Subjects:  []string{subject},
		Storage:   jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	consumer, err := js.CreateOrUpdateConsumer(ctx, "test-stream", jetstream.ConsumerConfig{
		Name:          "test-consumer",
		Durable:       "test-consumer",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: 1,
		MaxDeliver:    maxDeliver,
		AckWait:       ackWait,
	})
	require.NoError(t, err)

	return js, consumer
}

func TestInternalConsumeMessagesAcksOnSuccess(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	const subject = "test.success"
	js, consumer := newTestConsumer(t, nc, subject, 5, 500*time.Millisecond)

	var calls atomic.Int32
	//nolint:unparam // must satisfy the WALTaskHandler signature
	handler := func(_ context.Context, _ *WALTask) error {
		calls.Add(1)

		return nil
	}

	ctx := t.Context()
	go func() {
		_ = internalConsumeMessages(ctx, handler, consumer)
	}()

	payload, err := json.Marshal(WALTask{ClusterName: "c", WALName: "w"})
	require.NoError(t, err)
	_, err = js.Publish(ctx, subject, payload)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, 2*time.Second, 20*time.Millisecond)

	// Give a redelivery window a chance to (incorrectly) fire, then assert the
	// handler was invoked exactly once because the message was acked.
	time.Sleep(time.Second)
	assert.Equal(t, int32(1), calls.Load(), "successful message must be acked, not redelivered")
}

func TestInternalConsumeMessagesTerminatesInvalidMessage(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	const subject = "test.invalid"
	js, consumer := newTestConsumer(t, nc, subject, 5, 500*time.Millisecond)

	var calls atomic.Int32
	//nolint:unparam // must satisfy the WALTaskHandler signature
	handler := func(_ context.Context, _ *WALTask) error {
		calls.Add(1)

		return nil
	}

	ctx := t.Context()
	go func() {
		_ = internalConsumeMessages(ctx, handler, consumer)
	}()

	_, err = js.Publish(ctx, subject, []byte("not-json"))
	require.NoError(t, err)

	stream, err := js.Stream(ctx, "test-stream")
	require.NoError(t, err)

	// The invalid message must be terminated and removed from the work queue.
	require.Eventually(t, func() bool {
		info, infoErr := stream.Info(ctx)
		require.NoError(t, infoErr)

		return info.State.Msgs == 0
	}, 2*time.Second, 20*time.Millisecond)

	assert.Equal(t, int32(0), calls.Load(), "handler must not run for an invalid message")
}

func TestInternalConsumeMessagesRetriesUntilMaxDeliver(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	const subject = "test.retry"
	const maxDeliver = 3
	js, consumer := newTestConsumer(t, nc, subject, maxDeliver, 300*time.Millisecond)

	var calls atomic.Int32
	handler := func(_ context.Context, _ *WALTask) error {
		calls.Add(1)

		return errors.New("boom")
	}

	ctx := t.Context()
	go func() {
		_ = internalConsumeMessages(ctx, handler, consumer)
	}()

	payload, err := json.Marshal(WALTask{ClusterName: "c", WALName: "w"})
	require.NoError(t, err)
	_, err = js.Publish(ctx, subject, payload)
	require.NoError(t, err)

	// The handler is retried exactly MaxDeliver times and then no more: the
	// consumer stops redelivering once the attempt budget is exhausted.
	require.Eventually(t, func() bool {
		return calls.Load() == int32(maxDeliver)
	}, 5*time.Second, 50*time.Millisecond)

	// Give a further redelivery window a chance to fire and confirm the count
	// stays put.
	time.Sleep(time.Second)
	assert.Equal(t, int32(maxDeliver), calls.Load(),
		"a permanently failing message must be retried exactly MaxDeliver times")

	// Note: the message is neither acked nor terminated, so on a WorkQueue
	// stream it remains stored after the retry budget is spent. This documents
	// current behavior rather than asserting it is desirable.
	stream, err := js.Stream(ctx, "test-stream")
	require.NoError(t, err)
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), info.State.Msgs,
		"poison message is retained in the work queue after MaxDeliver")
}

// TestRunHeartbeatPreventsRedelivery verifies that the heartbeat keeps an
// in-flight message alive past its AckWait, and that redelivery resumes once
// the heartbeat stops.
func TestRunHeartbeatPreventsRedelivery(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	const subject = "test.heartbeat"
	const ackWait = 500 * time.Millisecond
	js, consumer := newTestConsumer(t, nc, subject, 5, ackWait)

	ctx := context.Background()
	_, err = js.Publish(ctx, subject, []byte("payload"))
	require.NoError(t, err)

	// Fetch the message so it becomes in-flight with its AckWait timer running.
	batch, err := consumer.Fetch(1)
	require.NoError(t, err)
	var msg jetstream.Msg
	for m := range batch.Messages() {
		msg = m
	}
	require.NoError(t, batch.Error())
	require.NotNil(t, msg)

	meta, err := msg.Metadata()
	require.NoError(t, err)
	require.Equal(t, uint64(1), meta.NumDelivered)

	// Run the heartbeat with an interval well below AckWait.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		runHeartbeat(heartbeatCtx, msg, 150*time.Millisecond)
		close(done)
	}()

	// While the heartbeat runs, the message must not be redelivered even though
	// we hold it for longer than AckWait.
	time.Sleep(3 * ackWait)
	emptyBatch, err := consumer.Fetch(1, jetstream.FetchMaxWait(200*time.Millisecond))
	require.NoError(t, err)
	for range emptyBatch.Messages() {
		t.Fatal("message was redelivered while the heartbeat was active")
	}
	require.NoError(t, emptyBatch.Error())

	// Stop the heartbeat and let AckWait expire: the message is now redelivered.
	heartbeatCancel()
	<-done

	require.Eventually(t, func() bool {
		redelivered, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(200*time.Millisecond))
		require.NoError(t, fetchErr)
		for m := range redelivered.Messages() {
			redeliveredMeta, metaErr := m.Metadata()
			require.NoError(t, metaErr)
			if redeliveredMeta.NumDelivered >= 2 {
				return true
			}
		}

		return false
	}, 3*time.Second, 50*time.Millisecond)
}

// TestDLQStreamsConfig verifies that New provisions the two dead-letter queue
// streams subscribed to the MAX_DELIVERIES advisory of their source consumers,
// with a retain-only (LimitsPolicy) retention so captured advisories are kept
// rather than consumed.
func TestDLQStreamsConfig(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	walInfo, err := conn.klioDLQWalStream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, klioDLQWalStreamName, walInfo.Config.Name)
	assert.Equal(t, jetstream.LimitsPolicy, walInfo.Config.Retention,
		"DLQ must retain advisories, not consume them like a work queue")
	assert.Equal(t, []string{
		server.JSAdvisoryConsumerMaxDeliveryExceedPre + "." +
			klioWalStreamName + "." + klioWalConsumerName,
	}, walInfo.Config.Subjects,
		"WAL DLQ must subscribe to the WAL consumer's MAX_DELIVERIES advisory")

	backupInfo, err := conn.klioDLQBackupStream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, klioDLQBackupStreamName, backupInfo.Config.Name)
	assert.Equal(t, jetstream.LimitsPolicy, backupInfo.Config.Retention)
	assert.Equal(t, []string{
		server.JSAdvisoryConsumerMaxDeliveryExceedPre + "." +
			klioBackupStreamName + "." + klioBackupConsumerName,
	}, backupInfo.Config.Subjects,
		"backup DLQ must subscribe to the backup consumer's MAX_DELIVERIES advisory")
}

// TestDLQCapturesMaxDeliveryAdvisory exercises the end-to-end capture path: a
// poison message that exhausts MaxDeliver must produce exactly one advisory in
// the DLQ stream, and that advisory's StreamSeq must point back to the original
// payload, which stays retained in the source work queue and can be fetched on
// demand.
func TestDLQCapturesMaxDeliveryAdvisory(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	const subject = "test.dlq"
	const maxDeliver = 3
	js, consumer := newTestConsumer(t, nc, subject, maxDeliver, 300*time.Millisecond)

	// A DLQ stream subscribed to the test consumer's MAX_DELIVERIES advisory,
	// mirroring how New wires the production DLQ streams.
	dlqSubject := server.JSAdvisoryConsumerMaxDeliveryExceedPre + ".test-stream.test-consumer"
	dlqStream, err := js.CreateOrUpdateStream(t.Context(), jetstream.StreamConfig{
		Name:      "test-dlq-stream",
		Retention: jetstream.LimitsPolicy,
		Subjects:  []string{dlqSubject},
		Storage:   jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	handler := func(_ context.Context, _ *WALTask) error {
		return errors.New("boom")
	}

	ctx := t.Context()
	go func() {
		_ = internalConsumeMessages(ctx, handler, consumer)
	}()

	payload, err := json.Marshal(WALTask{ClusterName: "c", WALName: "w"})
	require.NoError(t, err)
	_, err = js.Publish(ctx, subject, payload)
	require.NoError(t, err)

	// Exactly one advisory is captured once the delivery budget is spent.
	require.Eventually(t, func() bool {
		info, infoErr := dlqStream.Info(ctx)
		require.NoError(t, infoErr)

		return info.State.Msgs == 1
	}, 5*time.Second, 50*time.Millisecond)

	// Give a further window to confirm no duplicate advisory is captured.
	time.Sleep(time.Second)
	info, err := dlqStream.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), info.State.Msgs,
		"a single poison message must yield exactly one DLQ advisory")

	advMsg, err := dlqStream.GetMsg(ctx, 1)
	require.NoError(t, err)

	var advisory server.JSConsumerDeliveryExceededAdvisory
	require.NoError(t, json.Unmarshal(advMsg.Data, &advisory))
	assert.Equal(t, "test-stream", advisory.Stream)
	assert.Equal(t, "test-consumer", advisory.Consumer)
	assert.Equal(t, uint64(maxDeliver), advisory.Deliveries)

	// The advisory is only a pointer: the payload itself is fetched on demand
	// from the source stream by the StreamSeq it carries.
	origMsg, err := js.Stream(ctx, "test-stream")
	require.NoError(t, err)
	original, err := origMsg.GetMsg(ctx, advisory.StreamSeq)
	require.NoError(t, err)
	assert.JSONEq(t, string(payload), string(original.Data),
		"DLQ advisory StreamSeq must resolve to the retained original payload")
}
