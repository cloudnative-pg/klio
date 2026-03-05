package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Conn is a work queue as used by Klio.
type Conn struct {
	conn       *nats.Conn
	klioStream jetstream.Stream

	walConsumer    jetstream.Consumer
	backupConsumer jetstream.Consumer
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

	result.klioStream, err = js.CreateOrUpdateStream(
		ctx,
		jetstream.StreamConfig{
			Name:        "KLIO",
			Retention:   jetstream.InterestPolicy,
			Description: "Klio stream",
			Subjects: []string{
				"klio.*.wal",
				"klio.*.backup",
			},
			Storage: jetstream.FileStorage,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream stream: %w", err)
	}

	result.walConsumer, err = js.CreateOrUpdateConsumer(
		ctx,
		"KLIO",
		jetstream.ConsumerConfig{
			Name:          "klio-wal",
			Durable:       "klio-wal",
			AckPolicy:     jetstream.AckExplicitPolicy,
			FilterSubject: "klio.*.wal",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream WAL consumer: %w", err)
	}

	result.backupConsumer, err = js.CreateOrUpdateConsumer(
		ctx,
		"KLIO",
		jetstream.ConsumerConfig{
			Name:          "klio-backup",
			Durable:       "klio-backup",
			AckPolicy:     jetstream.AckExplicitPolicy,
			FilterSubject: "klio.*.backup",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("while creating or updating JetStream Base consumer: %w", err)
	}

	return result, nil
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
	walInfo, err := q.walConsumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("while getting WAL consumer info: %w", err)
	}

	if walInfo == nil {
		return nil, errors.New("WAL consumer info is nil")
	}

	backupInfo, err := q.backupConsumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("while getting backup consumer info: %w", err)
	}

	if backupInfo == nil {
		return nil, errors.New("backup consumer info is nil")
	}

	return &Status{
		PendingBackups: backupInfo.NumPending,
		PendingWALs:    walInfo.NumPending,
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
			if ackError := msg.Ack(); ackError != nil {
				logger.Error(err, "Error while acking an invalid queue message, this wrong message will be retried")
			}

			return
		}

		if err := handler(ctx, &task); err != nil {
			logger.Error(err, "Error while handling queue message, retrying", "task", &task)

			// no ack here, this message should be retried
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
