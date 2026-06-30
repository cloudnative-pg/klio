package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/jsm.go"
	"github.com/nats-io/jsm.go/api"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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

	mu      sync.Mutex
	streams map[string]*jsm.Stream
}

// NewStreamManager creates a new StreamManager instance using the provided NATS connection.
func NewStreamManager(conn *nats.Conn) (*StreamManager, error) {
	mgr, err := jsm.New(conn, jsm.WithTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}

	return &StreamManager{
		mgr:     mgr,
		streams: make(map[string]*jsm.Stream),
	}, nil
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

// configureStreams creates or updates all JetStream streams required by Klio.
func (m *StreamManager) configureStreams(ctx context.Context, js jetstream.JetStream) error {
	configs := []jetstream.StreamConfig{
		{
			Name:        klioWalStreamName,
			Retention:   jetstream.WorkQueuePolicy,
			Description: "Klio WAL Stream",
			Subjects:    []string{walSubject("*")},
			Storage:     jetstream.FileStorage,
		},
		{
			Name:        klioDLQWalStreamName,
			Retention:   jetstream.LimitsPolicy,
			Description: "Klio Dead Letter Queue WAL Stream",
			Subjects: []string{
				fmt.Sprintf("%s.%s.%s",
					server.JSAdvisoryConsumerMaxDeliveryExceedPre,
					klioWalStreamName,
					klioWalConsumerName),
			},
			Storage: jetstream.FileStorage,
		},
		{
			Name:              klioLatestUploadedWalStreamName,
			Retention:         jetstream.LimitsPolicy,
			Description:       "Klio Latest Uploaded WAL per Cluster Stream",
			Subjects:          []string{latestUploadedWalSubject("*")},
			Storage:           jetstream.FileStorage,
			MaxMsgsPerSubject: 1,
		},
		{
			Name:        klioBackupStreamName,
			Retention:   jetstream.WorkQueuePolicy,
			Description: "Klio Backup Stream",
			Subjects:    []string{backupSubject("*")},
			Storage:     jetstream.FileStorage,
		},
		{
			Name:        klioDLQBackupStreamName,
			Retention:   jetstream.LimitsPolicy,
			Description: "Klio Dead Letter Queue Backup Stream",
			Subjects: []string{
				fmt.Sprintf("%s.%s.%s",
					server.JSAdvisoryConsumerMaxDeliveryExceedPre,
					klioBackupStreamName,
					klioBackupConsumerName),
			},
			Storage: jetstream.FileStorage,
		},
	}

	for _, cfg := range configs {
		if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return fmt.Errorf("while creating or updating JetStream stream %q: %w", cfg.Name, err)
		}
	}

	return nil
}

// purgeWALDLQEntry removes the WAL dead-letter queue entry at the given stream
// sequence and releases the original message it references from the WAL
// work-queue stream.
func (m *StreamManager) purgeWALDLQEntry(_ context.Context, dlqSequence uint64) error {
	dlqStream, err := m.loadStreamOrNil(klioDLQWalStreamName)
	if err != nil {
		return err
	}
	sourceStream, err := m.loadStreamOrNil(klioWalStreamName)
	if err != nil {
		return err
	}
	if dlqStream == nil || sourceStream == nil {
		return nil
	}

	return m.purgeDLQEntryBySequence(dlqStream, sourceStream, dlqSequence)
}

// purgeBackupDLQEntries removes every backup dead-letter queue entry belonging to the
// given cluster. For each matching entry it deletes the DLQ advisory and releases the
// original failed message it references from the backup work-queue stream.
func (m *StreamManager) purgeBackupDLQEntries(ctx context.Context, clusterName string) error {
	dlqStream, err := m.loadStreamOrNil(klioDLQBackupStreamName)
	if err != nil {
		return err
	}
	sourceStream, err := m.loadStreamOrNil(klioBackupStreamName)
	if err != nil {
		return err
	}
	if dlqStream == nil || sourceStream == nil {
		return nil
	}

	failed, err := listFailedTasks[BackupTask](ctx, dlqStream, sourceStream, WithCluster(clusterName))
	if err != nil {
		return err
	}

	var errs []error
	for _, task := range failed {
		if err := m.purgeDLQEntryBySequence(dlqStream, sourceStream, task.Sequence); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// loadStreamOrNil loads a stream by name, caching the handle.
func (m *StreamManager) loadStreamOrNil(name string) (*jsm.Stream, error) {
	m.mu.Lock()
	cached, ok := m.streams[name]
	m.mu.Unlock()
	if ok {
		return cached, nil
	}

	stream, err := m.mgr.LoadStream(name)
	switch {
	case jsm.IsNatsError(err, uint16(server.JSStreamNotFoundErr)):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("loading stream %q: %w", name, err)
	}

	m.mu.Lock()
	m.streams[name] = stream
	m.mu.Unlock()

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

// purgeDLQEntryBySequence removes the dead-letter queue entry stored at dlqSeq
// and releases the original message it references from sourceStream.
func (m *StreamManager) purgeDLQEntryBySequence(
	dlqStream *jsm.Stream,
	sourceStream *jsm.Stream,
	dlqSeq uint64,
) error {
	advisoryMsg, err := dlqStream.ReadMessage(dlqSeq)
	if err != nil {
		if jsm.IsNatsError(err, uint16(server.JSNoMessageFoundErr)) {
			return nil
		}

		return fmt.Errorf("while fetching dead-letter queue entry at sequence %d: %w", dlqSeq, err)
	}

	var advisory server.JSConsumerDeliveryExceededAdvisory
	if err := json.Unmarshal(advisoryMsg.Data, &advisory); err != nil {
		return fmt.Errorf("while unmarshalling dead-letter queue advisory at sequence %d: %w", dlqSeq, err)
	}

	// Release the original failed message first. The message may already be
	// gone (e.g. drained by the work queue), so a missing original is treated
	// as already cleaned up rather than an error. Deleting the original before
	// the DLQ entry makes this function idempotent: if it is interrupted after
	// this point the DLQ entry still exists and a retry will skip the
	// already-deleted original and then complete the DLQ removal.
	// We read before deleting because DeleteMessageRequest returns the generic
	// JSStreamMsgDeleteFailedF (10057) when the message is absent, which would
	// mask real delete failures. ReadMessage returns the precise
	// JSNoMessageFoundErr (10037) that we can safely ignore.
	_, err = sourceStream.ReadMessage(advisory.StreamSeq)
	switch {
	case jsm.IsNatsError(err, uint16(server.JSNoMessageFoundErr)):
		// already gone, nothing to release
	case err != nil:
		return fmt.Errorf("while checking original message at sequence %d: %w", advisory.StreamSeq, err)
	default:
		if err := sourceStream.DeleteMessageRequest(api.JSApiMsgDeleteRequest{Seq: advisory.StreamSeq}); err != nil {
			return fmt.Errorf("while releasing original message at sequence %d: %w", advisory.StreamSeq, err)
		}
	}

	if err := dlqStream.DeleteMessageRequest(api.JSApiMsgDeleteRequest{Seq: dlqSeq}); err != nil {
		return fmt.Errorf("while deleting dead-letter queue entry at sequence %d: %w", dlqSeq, err)
	}

	return nil
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
