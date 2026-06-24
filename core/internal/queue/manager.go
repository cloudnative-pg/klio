package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/jsm.go"
	"github.com/nats-io/jsm.go/api"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// errIncompleteDLQListing indicates the DLQ pager stopped before reading every
// entry the stream reported, typically because the queue server was slow or
// unreachable mid-pagination. The pager surfaces a timeout the same way it
// surfaces a genuine end-of-stream, so this short-read check is what
// distinguishes the two and prevents a silently truncated listing.
var errIncompleteDLQListing = errors.New("incomplete DLQ listing")

// FailedTask represents a task that has failed and has been sent to the Dead Letter Queue (DLQ) stream.
type FailedTask[T clusterTask] struct {
	// Sequence is the sequence number of the message in the DLQ stream.
	Sequence uint64
	// Task is the decoded original task that failed.
	Task T
	// Timestamp is the time when the message was sent to the DLQ stream.
	Timestamp time.Time
}

type clusterTask interface {
	// Cluster returns the name of the cluster associated with this task.
	Cluster() string
}

// ListOption configures a DLQ listing call. Pass options to
// ListFailedWALTasks / ListFailedBackupTasks via functional options
// (e.g., WithCluster("foo")).
type ListOption func(*listConfig)

type listConfig struct {
	cluster string
}

// WithCluster restricts the returned DLQ entries to those whose original
// task belongs to the given cluster. An empty cluster name is a no-op.
func WithCluster(name string) ListOption {
	return func(c *listConfig) {
		c.cluster = name
	}
}

// StreamManager provides methods to interact with NATS streams.
type StreamManager struct {
	mgr *jsm.Manager
}

// NewStreamManager creates a new StreamManager instance using the provided NATS connection.
func NewStreamManager(conn *nats.Conn) (*StreamManager, error) {
	mgr, err := jsm.New(conn, jsm.WithTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}

	return &StreamManager{mgr: mgr}, nil
}

// GetStatus returns the current status of the task queue. Streams that have not
// been created yet are reported as empty.
func (m *StreamManager) GetStatus() (*Status, error) {
	walMsgs, err := m.pendingMsgs(klioWalStreamName)
	if err != nil {
		return nil, err
	}

	backupMsgs, err := m.pendingMsgs(klioBackupStreamName)
	if err != nil {
		return nil, err
	}

	return &Status{
		PendingBackups: backupMsgs,
		PendingWALs:    walMsgs,
	}, nil
}

// ListFailedWALTasks retrieves a list of failed WAL tasks from the Dead Letter Queue (DLQ) stream.
func (m *StreamManager) ListFailedWALTasks(ctx context.Context, opts ...ListOption) ([]FailedTask[WALTask], error) {
	walStream, err := m.loadStreamOrNil(klioWalStreamName)
	if err != nil {
		return nil, err
	}

	dlqWALStream, err := m.loadStreamOrNil(klioDLQWalStreamName)
	if err != nil {
		return nil, err
	}

	if walStream == nil || dlqWALStream == nil {
		return nil, nil
	}

	return listFailedTasks[WALTask](ctx, dlqWALStream, walStream, opts...)
}

// ListFailedBackupTasks retrieves a list of failed backup tasks from the Dead Letter Queue (DLQ) stream.
func (m *StreamManager) ListFailedBackupTasks(
	ctx context.Context,
	opts ...ListOption,
) ([]FailedTask[BackupTask], error) {
	backupStream, err := m.loadStreamOrNil(klioBackupStreamName)
	if err != nil {
		return nil, err
	}

	dlqBackupStream, err := m.loadStreamOrNil(klioDLQBackupStreamName)
	if err != nil {
		return nil, err
	}

	if backupStream == nil || dlqBackupStream == nil {
		return nil, nil
	}

	return listFailedTasks[BackupTask](ctx, dlqBackupStream, backupStream, opts...)
}

// loadStreamOrNil loads a stream by name. The queue streams are created lazily
// by the tier servers, so the admin server may query them before they exist; in
// that case the stream is reported as absent (nil, nil) rather than as an error.
func (m *StreamManager) loadStreamOrNil(name string) (*jsm.Stream, error) {
	stream, err := m.mgr.LoadStream(name)
	switch {
	case jsm.IsNatsError(err, uint16(server.JSStreamNotFoundErr)):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("loading stream %q: %w", name, err)
	}

	return stream, nil
}

// pendingMsgs returns the number of messages held by the named stream, or 0 if
// the stream does not exist yet.
func (m *StreamManager) pendingMsgs(name string) (uint64, error) {
	stream, err := m.loadStreamOrNil(name)
	if err != nil {
		return 0, err
	}
	if stream == nil {
		return 0, nil
	}

	info, err := stream.Information()
	if err != nil {
		return 0, fmt.Errorf("while getting %q stream info: %w", name, err)
	}
	if info == nil {
		return 0, fmt.Errorf("%q stream info is nil", name)
	}

	return info.State.Msgs, nil
}

func listFailedTasks[T clusterTask](
	ctx context.Context,
	dlqStream, taskStream *jsm.Stream,
	opts ...ListOption,
) ([]FailedTask[T], error) {
	var cfg listConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Snapshot how many entries the DLQ stream holds so listPager can detect a
	// short read and avoid returning a silently truncated listing.
	state, err := dlqStream.State()
	if err != nil {
		return nil, fmt.Errorf("while getting DLQ stream state: %w", err)
	}

	pgr, err := dlqStream.PageContents()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := pgr.Close(); err != nil {
			log.FromContext(ctx).Error(err, "while closing page reader for DLQ stream", "stream", dlqStream.Name())
		}
	}()

	return listPager[T](ctx, pgr, taskStream.ReadMessage, state.Msgs, cfg)
}

//nolint:cyclop
func listPager[T clusterTask](ctx context.Context,
	pgr *jsm.StreamPager,
	readMessage func(seq uint64) (*api.StoredMsg, error),
	expected uint64,
	cfg listConfig,
) ([]FailedTask[T], error) {
	tasks := make([]FailedTask[T], 0)

	// read counts the DLQ messages actually pulled from the pager (before any
	// orphan-skip or cluster filtering), so it can be compared against the
	// snapshot count to detect a short read.
	var read uint64

	for {
		dlqMsg, last, err := pgr.NextMsg(ctx)
		if err != nil {
			// A cancelled or expired caller context is a hard error, not an
			// end-of-stream signal.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, fmt.Errorf("while reading DLQ stream: %w", ctxErr)
			}
			if !last {
				return nil, fmt.Errorf("failed to read DLQ stream message: %w", err)
			}

			break
		}
		if dlqMsg == nil {
			break
		}
		read++

		dlqMetadata, err := jsm.ParseJSMsgMetadata(dlqMsg)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DLQ stream message metadata: %w", err)
		}

		var dlqTask server.JSConsumerDeliveryExceededAdvisory
		if err := json.Unmarshal(dlqMsg.Data, &dlqTask); err != nil {
			return nil, fmt.Errorf("failed to unmarshal DLQ stream message data: %w", err)
		}

		msg, err := readMessage(dlqTask.StreamSeq)
		switch {
		case err != nil && !jsm.IsNatsError(err, uint16(server.JSNoMessageFoundErr)):
			return nil, fmt.Errorf("failed to read original stream message: %w", err)
		case err != nil || msg == nil:
			// The DLQ advisory outlives the original message: once a failed
			// task is retried or the source stream is purged, the advisory
			// remains but its source message is gone (a not-found error, or a
			// nil message without error). Skip such orphaned entries instead
			// of failing the whole listing.
			log.FromContext(ctx).Info(
				"Skipping orphaned DLQ entry: original stream message not found",
				"streamSeq", dlqTask.StreamSeq,
			)

			continue
		}

		var task T
		if err := json.Unmarshal(msg.Data, &task); err != nil {
			return nil, fmt.Errorf("failed to unmarshal original stream message data: %w", err)
		}

		if cfg.cluster != "" && task.Cluster() != cfg.cluster {
			continue
		}

		tasks = append(tasks, FailedTask[T]{
			Sequence:  dlqMetadata.StreamSequence(),
			Task:      task,
			Timestamp: dlqMetadata.TimeStamp(),
		})
	}

	// The pager cannot distinguish a genuine end-of-stream from a timeout: both
	// terminate the loop above. If we read fewer entries than the stream
	// reported, the pager gave up early (slow/unreachable server) and the
	// listing would be silently truncated, so fail loudly instead.
	if read < expected {
		return nil, fmt.Errorf("%w: read %d of %d entries", errIncompleteDLQListing, read, expected)
	}

	slices.SortFunc(tasks, func(a, b FailedTask[T]) int {
		return b.Timestamp.Compare(a.Timestamp)
	})

	return tasks, nil
}
