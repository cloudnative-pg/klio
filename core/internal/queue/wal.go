package queue

import (
	"context"
	"fmt"
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
