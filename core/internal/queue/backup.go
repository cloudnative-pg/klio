package queue

import (
	"context"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// BackupTask is the structure that is sent on NATS Stream when
// a new backup has been received.
type BackupTask struct {
	// The name of the cluster
	ClusterName string `json:"clusterName"`

	// SendToTier2 indicates the backup must also be migrated to tier2.
	// When false the consumer only runs tier1 post-processing (verification
	// and maintenance) without touching tier2.
	SendToTier2 bool `json:"sendToTier2,omitempty"`

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
// when the context is canceled. After a successful handler run, all dead-letter
// queue entries for the backed-up cluster are purged.
func (q *Conn) ConsumeBackupReceivedMessages(ctx context.Context, handler BackupTaskHandler) error {
	wrapped := func(ctx context.Context, t *BackupTask, _ nats.Header) error {
		if err := handler(ctx, t); err != nil {
			return err
		}

		if err := q.purgeBackupDLQEntries(ctx, t.ClusterName); err != nil {
			log.FromContext(ctx).Error(
				err,
				"Failed to purge backup dead-letter queue entries after successful backup",
				"task", t,
			)
		}

		return nil
	}

	return internalConsumeMessages(ctx, wrapped, q.backupConsumer)
}
