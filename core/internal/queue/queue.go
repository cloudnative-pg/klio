package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// WALTask is the structure that is sent on NATS Stream when
// a new WAL has been received.
type WALTask struct {
	// ClusterName is the name of the cluster
	ClusterName string `json:"clusterName"`

	// WALName is the name of the WAL
	WALName string `json:"walName"`
}

// Conn is a work queue as used by Klio.
type Conn struct {
	conn *nats.Conn
}

// New creates a new queue client.
func New(natsConnection *nats.Conn) *Conn {
	return &Conn{
		conn: natsConnection,
	}
}

// EnsureSetup ensures that the NATS stream is correctly configured.
func (q *Conn) EnsureSetup(ctx context.Context) error {
	js, err := jetstream.New(q.conn)
	if err != nil {
		return fmt.Errorf("while creating JetStream instance: %w", err)
	}

	streamConfig := jetstream.StreamConfig{
		Name:        "KLIO",
		Retention:   jetstream.InterestPolicy,
		Description: "Klio stream",
		Subjects: []string{
			"klio.*.wal",
			"klio.*.backup",
		},
		Storage: jetstream.FileStorage,
	}

	_, err = js.CreateOrUpdateStream(ctx, streamConfig)
	if err != nil {
		return fmt.Errorf("while creating or updating JetStream stream: %w", err)
	}

	return nil
}

// NotifyWALReceived is called to notify the consumers that a new WAL
// is available in the Klio repository.
func (q *Conn) NotifyWALReceived(ctx context.Context, task *WALTask) error {
	js, err := jetstream.New(q.conn)
	if err != nil {
		return fmt.Errorf("while creating JetStream instance: %w", err)
	}

	rawContent, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("while marshalling task to JSON: %w", err)
	}

	_, err = js.Publish(ctx, fmt.Sprintf("klio.%s.wal", task.ClusterName), rawContent)
	if err != nil {
		return fmt.Errorf("while pushing message to the queue: %w", err)
	}

	return nil
}
