package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Conn is a work queue as used by Klio.
type Conn struct {
	conn                                  *nats.Conn
	klioWalStream                         jetstream.Stream
	klioBackupStream                      jetstream.Stream
	klioLatestUploadedWalPerClusterStream jetstream.Stream

	walConsumer    jetstream.Consumer
	backupConsumer jetstream.Consumer
}

const (
	klioBackupStreamName            = "klio-backup-stream"
	klioWalStreamName               = "klio-wal-stream"
	klioLatestUploadedWalStreamName = "klio-latest-uploaded-wal-per-cluster-stream"

	klioBackupConsumerName = "klio-backup-consumer"
	klioWalConsumerName    = "klio-wal-consumer"

	// legacyKlioStreamName is the name of the single combined stream used by
	// Klio before the WAL/backup split. It is removed automatically on
	// startup when empty so the queue volume does not accumulate stale
	// streams across upgrades.
	legacyKlioStreamName = "KLIO"
)

// heartbeatInterval is how often the consumer sends an InProgress ack while
// a handler is still working. It must be lower than the smallest backoff
// configured on the WAL and backup consumers, so JetStream never times out
// AckWait and redelivers a message that is still being processed.
const heartbeatInterval = 15 * time.Second

func backupSubject(clusterName string) string {
	return "klio.backup." + clusterName
}

func walSubject(clusterName string) string {
	return "klio.wal." + clusterName
}

func latestUploadedWalSubject(clusterName string) string {
	return "klio.latest-uploaded-wal." + clusterName
}

// New creates a new queue client.
func New(ctx context.Context, natsConnection *nats.Conn) (*Conn, error) {
	result := &Conn{
		conn: natsConnection,
	}

	js, err := jetstream.New(natsConnection)
	if err != nil {
		return nil, fmt.Errorf("while creating JetStream instance: %w", err)
	}

	// Drain and delete the legacy single KLIO stream before creating the
	// new ones, because its subjects (klio.*.wal, klio.*.backup) overlap
	// with the new layout (klio.wal.*, klio.backup.*).
	pending, err := drainLegacyStream(ctx, js)
	if err != nil {
		return nil, err
	}

	result.klioWalStream, err = js.CreateOrUpdateStream(
		ctx,
		jetstream.StreamConfig{
			Name:        klioWalStreamName,
			Retention:   jetstream.WorkQueuePolicy,
			Description: "Klio WAL Stream",
			Subjects: []string{
				walSubject("*"),
			},
			Storage: jetstream.FileStorage,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream WAL stream: %w", err)
	}

	result.klioLatestUploadedWalPerClusterStream, err = js.CreateOrUpdateStream(
		ctx,
		jetstream.StreamConfig{
			Name:              klioLatestUploadedWalStreamName,
			Retention:         jetstream.LimitsPolicy,
			Description:       "Klio Latest Uploaded WAL per Cluster Stream",
			Subjects:          []string{latestUploadedWalSubject("*")},
			Storage:           jetstream.FileStorage,
			MaxMsgsPerSubject: 1,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream latest-uploaded-WAL stream: %w", err)
	}

	result.klioBackupStream, err = js.CreateOrUpdateStream(
		ctx,
		jetstream.StreamConfig{
			Name:        klioBackupStreamName,
			Retention:   jetstream.WorkQueuePolicy,
			Description: "Klio Backup Stream",
			Subjects: []string{
				backupSubject("*"),
			},
			Storage: jetstream.FileStorage,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream backup stream: %w", err)
	}

	result.walConsumer, err = js.CreateOrUpdateConsumer(
		ctx,
		klioWalStreamName,
		jetstream.ConsumerConfig{
			Name:          klioWalConsumerName,
			Durable:       klioWalConsumerName,
			AckPolicy:     jetstream.AckExplicitPolicy,
			MaxAckPending: 1,
			MaxDeliver:    10,
			// BackOff drives both the redelivery delay and the per-delivery
			// AckWait, so we do not set AckWait explicitly: it would be
			// ignored. The effective ceiling is the last BackOff entry.
			BackOff: []time.Duration{
				30 * time.Second,
				1 * time.Minute,
				2 * time.Minute,
				5 * time.Minute,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream WAL consumer: %w", err)
	}

	result.backupConsumer, err = js.CreateOrUpdateConsumer(
		ctx,
		klioBackupStreamName,
		jetstream.ConsumerConfig{
			Name:          klioBackupConsumerName,
			Durable:       klioBackupConsumerName,
			AckPolicy:     jetstream.AckExplicitPolicy,
			MaxAckPending: 1,
			// Each backup task processes the full set of backups for a cluster,
			// so a failed task is naturally retried by the next one. Fewer attempts
			// here are acceptable.
			MaxDeliver: 5,
			// BackOff drives both the redelivery delay and the per-delivery
			// AckWait, so we do not set AckWait explicitly: it would be
			// ignored. The effective ceiling is the last BackOff entry.
			BackOff: []time.Duration{
				5 * time.Minute,
				10 * time.Minute,
				30 * time.Minute,
				1 * time.Hour,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream Base consumer: %w", err)
	}

	if err := republishLegacyMessages(ctx, js, pending); err != nil {
		return nil, err
	}

	return result, nil
}

// legacyMessage holds a message read from the pre-split KLIO stream so
// it can be republished to the new per-purpose streams after they are
// created.
type legacyMessage struct {
	subject string
	data    []byte
}

// drainLegacyStream reads any pending messages from the pre-split KLIO
// stream and then deletes the stream so the new streams (whose subjects
// overlap) can be created. The drained messages are returned so the
// caller can republish them once the new streams exist.
func drainLegacyStream(ctx context.Context, js jetstream.JetStream) ([]legacyMessage, error) {
	logger := log.FromContext(ctx)

	stream, err := js.Stream(ctx, legacyKlioStreamName)
	if err != nil {
		if errors.Is(err, jetstream.ErrStreamNotFound) {
			return nil, nil
		}

		return nil, fmt.Errorf("while looking up legacy KLIO stream: %w", err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("while getting legacy KLIO stream info: %w", err)
	}

	var pending []legacyMessage
	if info.State.Msgs > 0 {
		pending, err = readLegacyMessages(ctx, js, info.State.Msgs)
		if err != nil {
			return nil, fmt.Errorf("while reading legacy KLIO stream: %w", err)
		}
	}

	if err := js.DeleteStream(ctx, legacyKlioStreamName); err != nil {
		return nil, fmt.Errorf("while deleting legacy KLIO stream: %w", err)
	}

	if len(pending) > 0 {
		logger.Info(
			"Drained legacy KLIO stream, messages will be republished to the new streams",
			"pendingMessages", len(pending),
		)
	} else {
		logger.Info("Removed empty legacy KLIO stream from the queue volume")
	}

	return pending, nil
}

// readLegacyMessages fetches every message in the legacy stream via an
// ephemeral pull consumer with explicit ack (required by WorkQueue
// streams). Messages are not acked because the stream is deleted
// immediately after, which discards all sequences regardless.
func readLegacyMessages(
	ctx context.Context,
	js jetstream.JetStream,
	numMsgs uint64,
) ([]legacyMessage, error) {
	consumer, err := js.CreateOrUpdateConsumer(ctx, legacyKlioStreamName, jetstream.ConsumerConfig{
		AckPolicy:         jetstream.AckExplicitPolicy,
		InactiveThreshold: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("while creating ephemeral consumer: %w", err)
	}

	fetchCount, err := safecast.Convert[int](numMsgs)
	if err != nil {
		return nil, fmt.Errorf("while converting message count: %w", err)
	}

	batch, err := consumer.FetchNoWait(fetchCount)
	if err != nil {
		return nil, fmt.Errorf("while fetching legacy messages: %w", err)
	}

	pending := make([]legacyMessage, 0, fetchCount)
	for msg := range batch.Messages() {
		pending = append(pending, legacyMessage{
			subject: msg.Subject(),
			data:    msg.Data(),
		})
	}

	if err := batch.Error(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("while iterating legacy messages: %w", err)
	}

	return pending, nil
}

// republishLegacyMessages re-emits messages drained from the legacy KLIO
// stream onto the new per-purpose subjects.
func republishLegacyMessages(
	ctx context.Context,
	js jetstream.JetStream,
	pending []legacyMessage,
) error {
	if len(pending) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)

	for _, msg := range pending {
		newSubject, err := translateLegacySubject(msg.subject)
		if err != nil {
			logger.Error(err, "Skipping legacy message with unrecognized subject",
				"subject", msg.subject)
			continue
		}

		if _, err := js.Publish(ctx, newSubject, msg.data); err != nil {
			return fmt.Errorf(
				"while republishing legacy message from %q to %q: %w",
				msg.subject, newSubject, err,
			)
		}
	}

	logger.Info("Republished legacy KLIO messages to the new streams",
		"count", len(pending))

	return nil
}

// translateLegacySubject maps an old klio.<cluster>.{wal,backup} subject
// to its new klio.{wal,backup}.<cluster> counterpart.
func translateLegacySubject(subject string) (string, error) {
	parts := strings.Split(subject, ".")
	const expectedTokens = 3
	if len(parts) != expectedTokens || parts[0] != "klio" {
		return "", fmt.Errorf("unrecognized legacy subject %q", subject)
	}

	cluster, kind := parts[1], parts[2]
	switch kind {
	case "wal":
		return walSubject(cluster), nil
	case "backup":
		return backupSubject(cluster), nil
	default:
		return "", fmt.Errorf("unrecognized legacy subject kind %q in %q", kind, subject)
	}
}

// Status contains statistics about the task queue.
type Status struct {
	// PendingBackups is the number of backup synchronization tasks pending in the queue.
	PendingBackups uint64

	// PendingWALs is the number of WAL relay tasks pending in the queue.
	PendingWALs uint64
}

// GetStatus returns the current status of the task queue.
func (q *Conn) GetStatus(ctx context.Context) (*Status, error) {
	walInfo, err := q.klioWalStream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("while getting WAL stream info: %w", err)
	}

	if walInfo == nil {
		return nil, errors.New("WAL stream info is nil")
	}

	backupInfo, err := q.klioBackupStream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("while getting backup stream info: %w", err)
	}

	if backupInfo == nil {
		return nil, errors.New("backup stream info is nil")
	}

	return &Status{
		PendingBackups: backupInfo.State.Msgs,
		PendingWALs:    walInfo.State.Msgs,
	}, nil
}

// notifyMessage is called to send a message on the queue.
func (q *Conn) notifyMessage(ctx context.Context, subject string, task any) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Sending message", "subject", subject, "task", task)

	js, err := jetstream.New(q.conn)
	if err != nil {
		return fmt.Errorf("while creating JetStream instance: %w", err)
	}

	rawContent, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("while marshalling task to JSON: %w", err)
	}

	_, err = js.Publish(ctx, subject, rawContent)
	if err != nil {
		return fmt.Errorf("while pushing message to the queue: %w", err)
	}

	return nil
}

// internalConsumeMessages starts consuming messages and ends
// when the context is canceled.
func internalConsumeMessages[T any](
	ctx context.Context,
	handler func(ctx context.Context, t *T) error,
	consumer jetstream.Consumer,
) error {
	logger := log.FromContext(ctx)

	internalConsumer := func(msg jetstream.Msg) {
		var task T

		if err := json.Unmarshal(msg.Data(), &task); err != nil {
			logger := logger.WithValues("bytes", msg.Data())
			logger.Error(err, "Detected invalid queue message, skipping and removing it from the queue")
			if ackError := msg.TermWithReason("Invalid message data"); ackError != nil {
				logger.Error(ackError, "Failed to terminate invalid queue message")
			}

			return
		}

		meta, err := msg.Metadata()
		if err != nil {
			logger.Error(err, "Error while fetching message metadata")
			if ackError := msg.TermWithReason("Metadata cannot be extracted"); ackError != nil {
				logger.Error(ackError, "Failed to terminate invalid queue message")
			}

			return
		}

		if meta.NumDelivered > 1 {
			logger.Info("Queue message redelivered: previous delivery was not acknowledged",
				"task", &task,
				"numDelivered", meta.NumDelivered,
				"publishedAt", meta.Timestamp)
		}

		err = internalRunTaskHandler(ctx, msg, func() error {
			return handler(ctx, &task)
		})
		if err != nil {
			// We deliberately do not Nak the message: a plain Nak triggers
			// immediate redelivery and ignores the consumer's BackOff schedule.
			// Letting AckWait expire instead causes the server to use the
			// configured BackOff entry for the current delivery attempt.
			logger.Error(err, "Error while handling queue message, retrying", "task", &task)

			return
		}

		if err := msg.Ack(); err != nil {
			logger.Error(err, "Error while acking queue message, this message will be retried", "task", &task)
		}
	}

	consumerContext, consumerError := consumer.Consume(internalConsumer)
	if consumerError != nil {
		logger.Error(consumerError, "Generic error while consuming messages")

		return nil
	}

	<-ctx.Done()
	consumerContext.Stop()

	return nil
}

// internalRunTaskHandler runs handleMessage while a background goroutine sends
// InProgress acks for msg, and guarantees the goroutine has exited before
// returning so the caller can safely Ack/Term the message.
func internalRunTaskHandler(
	ctx context.Context,
	msg jetstream.Msg,
	handleMessage func() error,
) error {
	// Send InProgress acks while the handler is still working to prevent
	// redeliveries due to AckWait expiration for long-running tasks. The
	// interval is well below every configured consumer backoff.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	var heartbeatWG sync.WaitGroup

	// Defers run LIFO: cancel the context first, then wait for the goroutine
	// to drain. Using defers also covers the case where handleMessage panics.
	defer heartbeatWG.Wait()
	defer heartbeatCancel()

	heartbeatWG.Go(func() {
		runHeartbeat(heartbeatCtx, msg, heartbeatInterval)
	})

	return handleMessage()
}

func runHeartbeat(
	ctx context.Context,
	msg jetstream.Msg,
	interval time.Duration,
) {
	if interval <= 0 {
		return
	}

	logger := log.FromContext(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := msg.InProgress(); err != nil {
				logger.Info("InProgress ack failed", "err", err)
			}
		}
	}
}
