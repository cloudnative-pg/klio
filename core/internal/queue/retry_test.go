package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
