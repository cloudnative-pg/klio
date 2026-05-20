package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	return q.notifyMessage(ctx, walSubject(task.ClusterName), task)
}

// WALTaskHandler is called for every WAL task message that should be handled.
// If succeeds, the message is not retried.
type WALTaskHandler func(ctx context.Context, t *WALTask) error

// ConsumeWALReceivedMessages starts consuming the WAL received messages and ends
// when the context is canceled. After a successful handler run, the WAL is
// recorded as the latest uploaded WAL for its cluster.
func (q *Conn) ConsumeWALReceivedMessages(ctx context.Context, handler WALTaskHandler) error {
	wrapped := func(ctx context.Context, t *WALTask) error {
		if err := handler(ctx, t); err != nil {
			return err
		}

		if err := q.notifyMessage(
			ctx,
			latestUploadedWalSubject(t.ClusterName),
			t,
		); err != nil {
			log.FromContext(ctx).Error(
				err,
				"Failed to record latest uploaded WAL, retention safety may degrade",
				"task", t,
			)
		}

		return nil
	}

	return internalConsumeMessages(ctx, wrapped, q.walConsumer)
}

// GetLatestUploadedWAL returns the most recently uploaded WAL file name for
// the given cluster, or empty string if no WAL has been uploaded yet. This is
// used by retention to avoid deleting WALs that have not been transferred to
// tier2.
func (q *Conn) GetLatestUploadedWAL(ctx context.Context, clusterName string) (string, error) {
	subject := latestUploadedWalSubject(clusterName)

	msg, err := q.klioLatestUploadedWalPerClusterStream.GetLastMsgForSubject(ctx, subject)
	if err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return "", nil
		}

		return "", fmt.Errorf("while fetching latest uploaded WAL message: %w", err)
	}

	var task WALTask
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		return "", fmt.Errorf("while unmarshalling latest uploaded WAL message: %w", err)
	}

	return task.WALName, nil
}
