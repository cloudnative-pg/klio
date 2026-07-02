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

package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testWALName is the WAL name seeded by publishWALMessage.
const testWALName = "000000010000000000000001"

// publishWALMessage publishes a WAL task to the WAL work-queue stream and
// returns its assigned stream sequence. When retried is true, the message
// carries the dead-letter queue retry origin marker.
func publishWALMessage(t *testing.T, js jetstream.JetStream, clusterName string, retried bool) uint64 {
	t.Helper()

	data, err := json.Marshal(WALTask{ClusterName: clusterName, WALName: testWALName})
	require.NoError(t, err)

	msg := &nats.Msg{Subject: walSubject(clusterName), Data: data}
	if retried {
		msg.Header = nats.Header{}
		msg.Header.Set(TaskOriginHeaderKey, TaskOriginDLQRetry)
	}

	ack, err := js.PublishMsg(t.Context(), msg)
	require.NoError(t, err)

	return ack.Sequence
}

// publishBackupMessage publishes a backup task to the backup work-queue stream
// and returns its assigned stream sequence.
func publishBackupMessage(t *testing.T, js jetstream.JetStream, clusterName string) uint64 {
	t.Helper()

	data, err := json.Marshal(BackupTask{ClusterName: clusterName})
	require.NoError(t, err)

	ack, err := js.Publish(t.Context(), backupSubject(clusterName), data)
	require.NoError(t, err)

	return ack.Sequence
}

// seedDLQAdvisory publishes a synthetic max-deliveries advisory onto the
// dead-letter queue subject for the given stream/consumer, pointing at the
// original message sequence streamSeq.
func seedDLQAdvisory(t *testing.T, js jetstream.JetStream, streamName, consumerName string, streamSeq uint64) {
	t.Helper()

	advisory := server.JSConsumerDeliveryExceededAdvisory{
		Stream:    streamName,
		Consumer:  consumerName,
		StreamSeq: streamSeq,
	}
	data, err := json.Marshal(advisory)
	require.NoError(t, err)

	subject := fmt.Sprintf("%s.%s.%s",
		server.JSAdvisoryConsumerMaxDeliveryExceedPre, streamName, consumerName)
	_, err = js.Publish(t.Context(), subject, data)
	require.NoError(t, err)
}

// dlqMsgCount returns the number of messages currently stored in stream.
func dlqMsgCount(t *testing.T, stream jetstream.Stream) uint64 {
	t.Helper()

	info, err := stream.Info(t.Context())
	require.NoError(t, err)

	return info.State.Msgs
}

func TestPurgeWALDLQEntriesRemovesEntryAndReleasesOriginal(t *testing.T) {
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

	// The original failed WAL message stays in the work queue after exhausting
	// its delivery budget; its DLQ advisory points at that sequence.
	poisonSeq := publishWALMessage(t, js, "purge-cluster", false)
	seedDLQAdvisory(t, js, klioWalStreamName, klioWalConsumerName, poisonSeq)
	require.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQWalStreamName)))
	require.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioWalStreamName)))

	require.NoError(t, conn.purgeWALDLQEntries(ctx, "purge-cluster", testWALName))

	assert.Equal(t, uint64(0), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQWalStreamName)),
		"the dead-letter queue entry for the cluster and WAL must be purged")
	assert.Equal(t, uint64(0), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioWalStreamName)),
		"the original failed WAL message must be released from the work queue")
}

func TestPurgeWALDLQEntriesLeavesOtherWALsUntouched(t *testing.T) {
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

	// Two failed WALs for the same cluster: only the one matching the requested
	// WAL name must be purged.
	targetSeq := publishWALMessage(t, js, "multi-cluster", false)
	seedDLQAdvisory(t, js, klioWalStreamName, klioWalConsumerName, targetSeq)

	otherData, err := json.Marshal(WALTask{ClusterName: "multi-cluster", WALName: "000000010000000000000002"})
	require.NoError(t, err)
	otherAck, err := js.PublishMsg(ctx, &nats.Msg{Subject: walSubject("multi-cluster"), Data: otherData})
	require.NoError(t, err)
	seedDLQAdvisory(t, js, klioWalStreamName, klioWalConsumerName, otherAck.Sequence)

	require.Equal(t, uint64(2), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQWalStreamName)))

	require.NoError(t, conn.purgeWALDLQEntries(ctx, "multi-cluster", testWALName))

	assert.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQWalStreamName)),
		"only the entry matching the requested WAL name must be purged")
	assert.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioWalStreamName)),
		"the non-matching WAL's original message must be retained")
}

func TestPurgeWALDLQEntriesWithNoMatchingEntriesIsNoOp(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := context.Background()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	// A purge for a cluster/WAL with no dead-letter queue entries is a no-op.
	require.NoError(t, conn.purgeWALDLQEntries(ctx, "empty-cluster", testWALName))
}

func TestPurgeBackupDLQEntriesRemovesClusterEntries(t *testing.T) {
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

	// Two failures for cluster-a and one for cluster-b.
	for range 2 {
		seq := publishBackupMessage(t, js, "cluster-a")
		seedDLQAdvisory(t, js, klioBackupStreamName, klioBackupConsumerName, seq)
	}
	seqB := publishBackupMessage(t, js, "cluster-b")
	seedDLQAdvisory(t, js, klioBackupStreamName, klioBackupConsumerName, seqB)
	require.Equal(t, uint64(3), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQBackupStreamName)))
	require.Equal(t, uint64(3), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName)),
		"all three failed backup messages must still be in the work queue before the purge")

	require.NoError(t, conn.purgeBackupDLQEntries(ctx, "cluster-a"))

	assert.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQBackupStreamName)),
		"all of cluster-a's entries must be purged while cluster-b's is retained")
	assert.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioBackupStreamName)),
		"cluster-a's original messages must be released while cluster-b's is retained")
}

func TestWALConsumerPurgesDLQOnRetrySuccess(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := t.Context()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	// The retried task carries the DLQ retry origin marker. Its advisory points
	// at the task being processed; the purge runs inside the handler before the
	// message is acked, so its original is still present and is released
	// together with the advisory.
	retrySeq := publishWALMessage(t, js, "retry-cluster", true)
	seedDLQAdvisory(t, js, klioWalStreamName, klioWalConsumerName, retrySeq)
	require.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQWalStreamName)))

	handler := func(_ context.Context, _ *WALTask) error { return nil }
	go func() {
		_ = conn.ConsumeWALReceivedMessages(ctx, handler)
	}()

	require.Eventually(t, func() bool {
		info, infoErr := streamHandle(ctx, t, conn.conn, klioDLQWalStreamName).Info(ctx)
		return infoErr == nil && info.State.Msgs == 0
	}, 5*time.Second, 50*time.Millisecond,
		"a successful retry must purge the dead-letter queue entry for the cluster and WAL")
}

func TestWALConsumerSkipsDLQWithoutRetryMarker(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := t.Context()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	poisonSeq := publishWALMessage(t, js, "normal-cluster", false)
	seedDLQAdvisory(t, js, klioWalStreamName, klioWalConsumerName, poisonSeq)

	handler := func(_ context.Context, _ *WALTask) error { return nil }
	go func() {
		_ = conn.ConsumeWALReceivedMessages(ctx, handler)
	}()

	// Wait until the non-retry message has been fully handled and acked (its
	// work-queue entry drains), which guarantees the post-success path ran.
	require.Eventually(t, func() bool {
		info, infoErr := streamHandle(ctx, t, conn.conn, klioWalStreamName).Info(ctx)
		return infoErr == nil && info.State.Msgs == 0
	}, 5*time.Second, 25*time.Millisecond,
		"the non-retry WAL message must be processed")

	// A non-retry WAL success must not touch the dead-letter queue.
	assert.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQWalStreamName)),
		"a non-retry WAL success must not purge any dead-letter queue entry")
}

func TestBackupConsumerPurgesDLQOnSuccess(t *testing.T) {
	ns, url := startNATSServer(t)
	defer ns.Shutdown()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	ctx := t.Context()
	conn, err := New(ctx, nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	seq := publishBackupMessage(t, js, "backup-cluster")
	seedDLQAdvisory(t, js, klioBackupStreamName, klioBackupConsumerName, seq)
	require.Equal(t, uint64(1), dlqMsgCount(t, streamHandle(ctx, t, conn.conn, klioDLQBackupStreamName)))

	handler := func(_ context.Context, _ *BackupTask) error { return nil }
	go func() {
		_ = conn.ConsumeBackupReceivedMessages(ctx, handler)
	}()

	require.Eventually(t, func() bool {
		info, infoErr := streamHandle(ctx, t, conn.conn, klioDLQBackupStreamName).Info(ctx)
		return infoErr == nil && info.State.Msgs == 0
	}, 5*time.Second, 50*time.Millisecond,
		"a successful backup must purge the cluster's dead-letter queue entries")
}
