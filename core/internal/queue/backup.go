package queue

import (
	"context"
	"fmt"
)

// BackupTask is the structure that is sent on NATS Stream when
// a new backup has been received.
type BackupTask struct {
	// The name of the cluster
	ClusterName string `json:"clusterName"`

	// The kopia source to be synchronized to tier2
	Sources []string `json:"sources"`
}

// NotifyBackupReceived is called to notify the consumers that a new backup
// has been uploaded.
func (q *Conn) NotifyBackupReceived(ctx context.Context, task *BackupTask) error {
	return q.notifyMessage(ctx, fmt.Sprintf("klio.%s.backup", task.ClusterName), task)
}

// BackupTaskHandler is called for every WAL task message that should be handled.
// If succeeds, the message is not retried.
type BackupTaskHandler func(ctx context.Context, t *BackupTask) error

// ConsumeBackupReceivedMessages starts consuming the WAL received messages and ends
// when the context is canceled.
func (q *Conn) ConsumeBackupReceivedMessages(ctx context.Context, handler BackupTaskHandler) error {
	return internalConsumeMessages(ctx, handler, q.backupConsumer)
}
