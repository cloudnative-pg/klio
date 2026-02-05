package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go/jetstream"
)

// WALTask is the structure that is sent on NATS Stream when
// a new WAL has been received.
type WALTask struct {
	// ClusterName is the name of the cluster
	ClusterName string `json:"clusterName"`

	// WALName if the name of the WAL
	WALName string `json:"walName"`
}

// NotifyWALReceived is called to notify the consumers that a new WAL
// is available in the Klio repository.
func (q *Conn) NotifyWALReceived(ctx context.Context, task *WALTask) error {
	return q.notifyMessage(ctx, fmt.Sprintf("klio.%s.wal", task.ClusterName), task)
}

// WALTaskHandler is called for every WAL task message that should be handled.
// If succeeds, the message is not retried.
type WALTaskHandler func(ctx context.Context, t *WALTask) error

// ConsumeWALReceivedMessages starts consuming the WAL received messages and ends
// when the context is canceled.
func (q *Conn) ConsumeWALReceivedMessages(ctx context.Context, handler WALTaskHandler) error {
	return internalConsumeMessages(ctx, handler, q.walConsumer)
}

// GetOldestPendingWAL returns the oldest WAL file name that is still pending
// in the queue for the given cluster. Returns empty string if no pending WALs.
// This is used to prevent tier1 WAL retention from deleting WAL files that
// haven't been transferred to tier2 yet.
//
// This checks both NumPending (messages not yet delivered) and NumAckPending
// (messages delivered but not yet acknowledged, i.e., in-flight).
func (q *Conn) GetOldestPendingWAL(ctx context.Context, clusterName string) (string, error) {
	logger := log.FromContext(ctx)

	// Get consumer info to check if there are pending messages
	info, err := q.walConsumer.Info(ctx)
	if err != nil {
		return "", fmt.Errorf("while getting WAL consumer info: %w", err)
	}

	// Total messages that haven't been fully processed:
	// - NumPending: waiting to be delivered
	// - NumAckPending: delivered but not yet acknowledged (being processed)
	numAckPending, err := safecast.Convert[uint64](info.NumAckPending)
	if err != nil {
		return "", fmt.Errorf("while converting NumAckPending: %w", err)
	}
	totalPending := info.NumPending + numAckPending

	// No pending messages
	if totalPending == 0 {
		logger.Debug("No pending WAL messages in queue")
		return "", nil
	}

	logger.Info("Checking pending WAL messages",
		"numPending", info.NumPending,
		"numAckPending", info.NumAckPending,
		"totalPending", totalPending)

	return q.findOldestPendingWALForCluster(ctx, clusterName, totalPending)
}

// maxFetchCount is the maximum number of messages to fetch when scanning
// for the oldest pending WAL. This prevents memory issues if the queue
// has accumulated a very large number of messages.
const maxFetchCount = 10000

// findOldestPendingWALForCluster scans pending messages in the queue to find
// the oldest WAL file for a specific cluster.
func (q *Conn) findOldestPendingWALForCluster(
	ctx context.Context,
	clusterName string,
	numPending uint64,
) (string, error) {
	logger := log.FromContext(ctx)
	subject := fmt.Sprintf("klio.%s.wal", clusterName)

	// Create a temporary consumer to peek at messages for this specific cluster
	js, err := jetstream.New(q.conn)
	if err != nil {
		return "", fmt.Errorf("while creating JetStream instance: %w", err)
	}

	// Use OrderedConsumer to peek at messages without affecting the main consumer.
	// This creates an ephemeral consumer that will be cleaned up automatically.
	orderedConsumer, err := js.OrderedConsumer(ctx, "KLIO", jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{subject},
	})
	if err != nil {
		return "", fmt.Errorf("while creating ordered consumer for peeking: %w", err)
	}

	// Safe conversion from uint64 to int, capped at maxFetchCount
	fetchCount, err := safecast.Convert[int](numPending)
	if err != nil {
		return "", fmt.Errorf("while converting numPending to int: %w", err)
	}
	if fetchCount > maxFetchCount {
		// If there are more pending messages than maxFetchCount, we might miss
		// older WAL files that have been retried and are now later in the queue.
		// In this case, we err on the side of caution by not deleting any WALs.
		logger.Warning("Too many pending WAL messages to safely scan, skipping WAL retention",
			"numPending", numPending, "maxFetchCount", maxFetchCount)
		// Return a synthetic "very old" WAL name to prevent any deletion.
		// WAL names start with timeline, so "0" will be older than any real WAL.
		return "000000000000000000000000", nil
	}

	// Fetch messages in batches to find the oldest WAL for this cluster
	msgBatch, err := orderedConsumer.FetchNoWait(fetchCount)
	if err != nil {
		return "", fmt.Errorf("while fetching messages: %w", err)
	}

	oldestWAL := extractOldestWALFromMessages(logger, msgBatch, clusterName)

	if oldestWAL != "" {
		logger.Info("Found oldest pending WAL in queue", "clusterName", clusterName, "oldestWAL", oldestWAL)
	}

	return oldestWAL, nil
}

// extractOldestWALFromMessages iterates through messages and finds the oldest WAL
// for a given cluster.
func extractOldestWALFromMessages(
	logger log.Logger,
	msgBatch jetstream.MessageBatch,
	clusterName string,
) string {
	oldestWAL := ""

	for msg := range msgBatch.Messages() {
		var task WALTask
		if err := json.Unmarshal(msg.Data(), &task); err != nil {
			logger.Debug("Skipping invalid message while peeking", "error", err)
			continue
		}

		// Only consider messages for this cluster
		if task.ClusterName != clusterName {
			continue
		}

		// Track the oldest WAL (lexicographically smallest)
		if oldestWAL == "" || strings.Compare(task.WALName, oldestWAL) < 0 {
			oldestWAL = task.WALName
		}
	}

	if err := msgBatch.Error(); err != nil {
		logger.Debug("Error while fetching messages", "error", err)
	}

	return oldestWAL
}
