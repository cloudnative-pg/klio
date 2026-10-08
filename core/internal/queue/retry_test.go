package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// seedFailedWAL publishes an original WAL task to the WAL work-queue stream and
// a matching dead-letter queue advisory, simulating a WAL that has exhausted
// its delivery budget.
func seedFailedWAL(t *testing.T, js jetstream.JetStream, clusterName, walName string) {
	t.Helper()

	data, err := json.Marshal(WALTask{ClusterName: clusterName, WALName: walName})
	require.NoError(t, err)

	ack, err := js.PublishMsg(t.Context(), &nats.Msg{Subject: walSubject(clusterName), Data: data})
	require.NoError(t, err)

	seedDLQAdvisory(t, js, klioWalStreamName, klioWalConsumerName, ack.Sequence)
}

// retriedWALs returns the set of WAL tasks re-enqueued onto the WAL work-queue
// stream, identified by the DLQ retry origin marker.
func retriedWALs(t *testing.T, stream jetstream.Stream) map[WALTask]struct{} {
	t.Helper()

	info, err := stream.Info(t.Context())
	require.NoError(t, err)

	out := make(map[WALTask]struct{})
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq && seq != 0; seq++ {
		msg, err := stream.GetMsg(t.Context(), seq)
		if err != nil {
			// Sequences may be absent (e.g. deleted); skip them.
			continue
		}
		if msg.Header.Get(TaskOriginHeaderKey) != TaskOriginDLQRetry {
			continue
		}

		var task WALTask
		require.NoError(t, json.Unmarshal(msg.Data, &task))
		out[task] = struct{}{}
	}

	return out
}

func TestRetryFailedWALTasksRetriesAllClusters(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-b", "000000010000000000000002")

	require.NoError(t, conn.RetryFailedWALTasks(ctx))

	retried := retriedWALs(t, streamHandle(ctx, t, conn.conn, klioWalStreamName))
	assert.Equal(t, map[WALTask]struct{}{
		{ClusterName: "cluster-a", WALName: "000000010000000000000001"}: {},
		{ClusterName: "cluster-b", WALName: "000000010000000000000002"}: {},
	}, retried)
}

func TestRetryFailedWALTasksRetriesSingleCluster(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-b", "000000010000000000000002")

	require.NoError(t, conn.RetryFailedWALTasks(ctx, WithCluster("cluster-a")))

	retried := retriedWALs(t, streamHandle(ctx, t, conn.conn, klioWalStreamName))
	assert.Equal(t, map[WALTask]struct{}{
		{ClusterName: "cluster-a", WALName: "000000010000000000000001"}: {},
	}, retried, "only the requested cluster's failed WAL must be retried")
}

func TestRetryFailedWALTasksRetriesSpecificWALs(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-a", "000000010000000000000002")
	seedFailedWAL(t, js, "cluster-a", "000000010000000000000003")

	require.NoError(t, conn.RetryFailedWALTasks(
		ctx,
		WithCluster("cluster-a"),
		WithWALs("000000010000000000000001", "000000010000000000000003"),
	))

	retried := retriedWALs(t, streamHandle(ctx, t, conn.conn, klioWalStreamName))
	assert.Equal(t, map[WALTask]struct{}{
		{ClusterName: "cluster-a", WALName: "000000010000000000000001"}: {},
		{ClusterName: "cluster-a", WALName: "000000010000000000000003"}: {},
	}, retried, "only the requested WAL names must be retried")
}

func TestRetryFailedWALTasksSkipsUnknownWALs(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")

	// An unknown WAL name is silently ignored; the known one is still retried.
	require.NoError(t, conn.RetryFailedWALTasks(
		ctx,
		WithCluster("cluster-a"),
		WithWALs("000000010000000000000001", "000000019999999999999999"),
	))

	retried := retriedWALs(t, streamHandle(ctx, t, conn.conn, klioWalStreamName))
	assert.Equal(t, map[WALTask]struct{}{
		{ClusterName: "cluster-a", WALName: "000000010000000000000001"}: {},
	}, retried, "only the WAL names that matched a failed task must be retried")
}

// seedFailedBackup publishes an original backup task to the backup work-queue
// stream and a matching dead-letter queue advisory, simulating a backup that
// has exhausted its delivery budget.
func seedFailedBackup(t *testing.T, js jetstream.JetStream, clusterName string) {
	t.Helper()

	seq := publishBackupMessage(t, js, clusterName)
	seedDLQAdvisory(t, js, klioBackupStreamName, klioBackupConsumerName, seq)
}

// seedFailedBackupTask publishes the given backup task to the work-queue
// stream and dead-letters it, simulating a backup that exhausted its
// delivery budget.
func seedFailedBackupTask(t *testing.T, js jetstream.JetStream, task BackupTask) {
	t.Helper()

	seq := publishBackupTask(t, js, task)
	seedDLQAdvisory(t, js, klioBackupStreamName, klioBackupConsumerName, seq)
}

// retriedBackupClusters returns, per cluster, the number of backup tasks
// re-enqueued onto the backup work-queue stream, identified by the DLQ retry
// origin marker.
func retriedBackupClusters(t *testing.T, stream jetstream.Stream) map[string]int {
	t.Helper()

	info, err := stream.Info(t.Context())
	require.NoError(t, err)

	out := make(map[string]int)
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq && seq != 0; seq++ {
		msg, err := stream.GetMsg(t.Context(), seq)
		if err != nil {
			// Sequences may be absent (e.g. deleted); skip them.
			continue
		}
		if msg.Header.Get(TaskOriginHeaderKey) != TaskOriginDLQRetry {
			continue
		}

		var task BackupTask
		require.NoError(t, json.Unmarshal(msg.Data, &task))
		out[task.ClusterName]++
	}

	return out
}

func TestRetryFailedBackupTasksRetriesAllClusters(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-b")

	require.NoError(t, conn.RetryFailedBackupTasks(ctx))

	retried := retriedBackupClusters(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName))
	assert.Equal(t, map[string]int{"cluster-a": 1, "cluster-b": 1}, retried)
}

func TestRetryFailedBackupTasksRetriesSingleCluster(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-b")

	require.NoError(t, conn.RetryFailedBackupTasks(ctx, WithCluster("cluster-a")))

	retried := retriedBackupClusters(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName))
	assert.Equal(t, map[string]int{"cluster-a": 1}, retried,
		"only the requested cluster's failed backup must be retried")
}

func TestRetryFailedBackupTasksDeduplicatesByCluster(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	// Two failed backups for the same cluster must collapse into a single retry.
	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-a")

	require.NoError(t, conn.RetryFailedBackupTasks(ctx))

	retried := retriedBackupClusters(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName))
	assert.Equal(t, map[string]int{"cluster-a": 1}, retried,
		"multiple failed backups for one cluster must be retried only once")
}

// retriedBackupTasks returns every backup task body re-enqueued onto the
// backup work-queue stream, identified by the DLQ retry origin marker.
func retriedBackupTasks(t *testing.T, stream jetstream.Stream) []BackupTask {
	t.Helper()

	info, err := stream.Info(t.Context())
	require.NoError(t, err)

	var out []BackupTask
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq && seq != 0; seq++ {
		msg, err := stream.GetMsg(t.Context(), seq)
		if err != nil {
			// Sequences may be absent (e.g. deleted); skip them.
			continue
		}
		if msg.Header.Get(TaskOriginHeaderKey) != TaskOriginDLQRetry {
			continue
		}

		var task BackupTask
		require.NoError(t, json.Unmarshal(msg.Data, &task))
		out = append(out, task)
	}

	return out
}

func TestRetryFailedBackupTasksOrsSendToTier2(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	// Two failed backups for the same cluster disagree on SendToTier2: the
	// merged retry must ask for tier2, since the relay is additive/idempotent.
	seedFailedBackupTask(t, js, BackupTask{ClusterName: "cluster-a", SendToTier2: false})
	seedFailedBackupTask(t, js, BackupTask{ClusterName: "cluster-a", SendToTier2: true})

	require.NoError(t, conn.RetryFailedBackupTasks(ctx))

	tasks := retriedBackupTasks(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName))
	require.Len(t, tasks, 1)
	assert.True(t, tasks[0].SendToTier2)
}

func TestRetryFailedBackupTasksKeepsAgreeingPolicy(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	keepLatest := 5
	retention := &kopia.RetentionPolicy{KeepLatest: &keepLatest}
	compression := &kopia.CompressionPolicy{Algorithm: "zstd"}

	// Both failed entries agree on the same policy values (one leaves them
	// unset): the merged retry must keep them rather than blank them out.
	seedFailedBackupTask(t, js, BackupTask{ClusterName: "cluster-a"})
	seedFailedBackupTask(t, js, BackupTask{
		ClusterName:            "cluster-a",
		Tier2RetentionPolicy:   retention,
		Tier2CompressionPolicy: compression,
	})

	require.NoError(t, conn.RetryFailedBackupTasks(ctx))

	tasks := retriedBackupTasks(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName))
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].Tier2RetentionPolicy)
	require.NotNil(t, tasks[0].Tier2CompressionPolicy)
	assert.Equal(t, keepLatest, *tasks[0].Tier2RetentionPolicy.KeepLatest)
	assert.Equal(t, "zstd", tasks[0].Tier2CompressionPolicy.Algorithm)
}

func TestRetryFailedBackupTasksBlanksConflictingPolicy(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	keepLatest5 := 5
	keepLatest10 := 10

	// The two failed entries for the same cluster disagree on the tier2
	// retention/compression policy: the merged retry must not overwrite the
	// tier2 source's existing policy with an arbitrary pick, so it blanks
	// both fields instead.
	seedFailedBackupTask(t, js, BackupTask{
		ClusterName:            "cluster-a",
		Tier2RetentionPolicy:   &kopia.RetentionPolicy{KeepLatest: &keepLatest5},
		Tier2CompressionPolicy: &kopia.CompressionPolicy{Algorithm: "zstd"},
	})
	seedFailedBackupTask(t, js, BackupTask{
		ClusterName:            "cluster-a",
		Tier2RetentionPolicy:   &kopia.RetentionPolicy{KeepLatest: &keepLatest10},
		Tier2CompressionPolicy: &kopia.CompressionPolicy{Algorithm: "gzip"},
	})

	require.NoError(t, conn.RetryFailedBackupTasks(ctx))

	tasks := retriedBackupTasks(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName))
	require.Len(t, tasks, 1)
	assert.Nil(t, tasks[0].Tier2RetentionPolicy)
	assert.Nil(t, tasks[0].Tier2CompressionPolicy)
}

func TestRetryFailedBackupTasksRejectsWALFilter(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seedFailedBackup(t, js, "cluster-a")

	// BackupTask has no individual WAL name to filter on: WithWALs must be
	// rejected rather than silently ignored.
	err = conn.RetryFailedBackupTasks(ctx, WithWALs("000000010000000000000001"))
	require.ErrorIs(t, err, errWALFilterUnsupported)
}

// discardTestEnv bundles what a discard test needs: a queue connection and a
// JetStream handle for seeding failed tasks.
type discardTestEnv struct {
	conn *Conn
	js   jetstream.JetStream
}

// newDiscardTestEnv starts an embedded NATS server and connects a queue to it.
// The server and the NATS connection are torn down when the test ends.
func newDiscardTestEnv(t *testing.T) discardTestEnv {
	t.Helper()

	ns, url := startNATSServer(t)
	t.Cleanup(ns.Shutdown)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	conn, err := New(t.Context(), nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	return discardTestEnv{conn: conn, js: js}
}

// assertQueueCounts checks the number of messages left in a work-queue stream
// and in its dead-letter queue stream.
func assertQueueCounts(
	t *testing.T,
	conn *Conn,
	workStream, dlqStream string,
	wantWork, wantDLQ uint64,
) {
	t.Helper()

	assert.Equal(t, wantDLQ, dlqMsgCount(t, streamHandle(t.Context(), t, conn.conn, dlqStream)),
		"unexpected number of dead-letter queue entries")
	assert.Equal(t, wantWork, dlqMsgCount(t, streamHandle(t.Context(), t, conn.conn, workStream)),
		"unexpected number of messages left in the work queue")
}

func TestDiscardFailedWALTasksDiscardsAllClusters(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-b", "000000010000000000000002")

	require.NoError(t, conn.DiscardFailedWALTasks(ctx))

	assertQueueCounts(t, conn, klioWalStreamName, klioDLQWalStreamName, 0, 0)
	assert.Empty(t, retriedWALs(t, streamHandle(ctx, t, conn.conn, klioWalStreamName)),
		"discarded WALs must not be re-enqueued")
}

func TestDiscardFailedWALTasksDiscardsSingleCluster(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-b", "000000010000000000000002")

	require.NoError(t, conn.DiscardFailedWALTasks(ctx, WithCluster("cluster-a")))

	assertQueueCounts(t, conn, klioWalStreamName, klioDLQWalStreamName, 1, 1)

	remaining, err := conn.ListFailedWALTasks(ctx)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "cluster-b", remaining[0].Task.ClusterName,
		"only the requested cluster's failed WAL must be discarded")
}

func TestDiscardFailedWALTasksDiscardsSpecificWALs(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-a", "000000010000000000000002")
	seedFailedWAL(t, js, "cluster-a", "000000010000000000000003")

	require.NoError(t, conn.DiscardFailedWALTasks(
		ctx,
		WithCluster("cluster-a"),
		WithWALs("000000010000000000000001", "000000010000000000000003"),
	))

	assertQueueCounts(t, conn, klioWalStreamName, klioDLQWalStreamName, 1, 1)

	remaining, err := conn.ListFailedWALTasks(ctx)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "000000010000000000000002", remaining[0].Task.WALName,
		"only the requested WAL names must be discarded")
}

func TestDiscardFailedWALTasksSkipsUnknownWALs(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")

	// An unknown WAL name is silently ignored; the known one is still discarded.
	require.NoError(t, conn.DiscardFailedWALTasks(
		ctx,
		WithCluster("cluster-a"),
		WithWALs("000000010000000000000001", "000000019999999999999999"),
	))

	assertQueueCounts(t, conn, klioWalStreamName, klioDLQWalStreamName, 0, 0)
}

func TestDiscardFailedBackupTasksDiscardsAllClusters(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-b")

	require.NoError(t, conn.DiscardFailedBackupTasks(ctx))

	assertQueueCounts(t, conn, klioBackupStreamName, klioDLQBackupStreamName, 0, 0)
	assert.Empty(t, retriedBackupClusters(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName)),
		"discarded backups must not be re-enqueued")
}

func TestDiscardFailedBackupTasksDiscardsSingleCluster(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-b")

	require.NoError(t, conn.DiscardFailedBackupTasks(ctx, WithCluster("cluster-a")))

	assertQueueCounts(t, conn, klioBackupStreamName, klioDLQBackupStreamName, 1, 1)

	remaining, err := conn.ListFailedBackupTasks(ctx)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "cluster-b", remaining[0].Task.ClusterName,
		"only the requested cluster's failed backup must be discarded")
}

func TestDiscardFailedBackupTasksDiscardsEveryEntryOfCluster(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	// Unlike retry, which collapses them into one task, discard must release
	// every failed backup entry of the cluster.
	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-a")

	require.NoError(t, conn.DiscardFailedBackupTasks(ctx))

	assertQueueCounts(t, conn, klioBackupStreamName, klioDLQBackupStreamName, 0, 0)
}

func TestDiscardFailedBackupTasksRejectsWALFilter(t *testing.T) {
	env := newDiscardTestEnv(t)
	ctx, conn, js := t.Context(), env.conn, env.js

	seedFailedBackup(t, js, "cluster-a")

	// BackupTask has no individual WAL name to filter on: WithWALs must be
	// rejected rather than silently ignored, and nothing must be discarded.
	err := conn.DiscardFailedBackupTasks(ctx, WithWALs("000000010000000000000001"))
	require.ErrorIs(t, err, errWALFilterUnsupported)

	assertQueueCounts(t, conn, klioBackupStreamName, klioDLQBackupStreamName, 1, 1)
}
