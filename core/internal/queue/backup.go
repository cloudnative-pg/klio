package queue

import (
	"context"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// BackupTask is the structure that is sent on NATS Stream when
// a new backup has been received.
type BackupTask struct {
	// The name of the cluster
	ClusterName string `json:"clusterName"`

	// The retention policy to apply to tier2.
	Tier2RetentionPolicy *kopia.RetentionPolicy `json:"tier2RetentionPolicy,omitzero"`
}

// Cluster returns the name of the cluster associated with this task.
func (t BackupTask) Cluster() string {
	return t.ClusterName
}

// NotifyBackupReceived is called to notify the consumers that a new backup
// has been uploaded.
func (q *Conn) NotifyBackupReceived(ctx context.Context, task *BackupTask) error {
	return q.notifyMessage(ctx, backupSubject(task.ClusterName), task)
}

// BackupTaskHandler is called for every backup task message that should be handled.
// If succeeds, the message is not retried.
type BackupTaskHandler func(ctx context.Context, t *BackupTask) error

// ConsumeBackupReceivedMessages starts consuming the backup received messages and ends
// when the context is canceled.
func (q *Conn) ConsumeBackupReceivedMessages(ctx context.Context, handler BackupTaskHandler) error {
	return internalConsumeMessages(ctx, handler, q.backupConsumer)
}
