/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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

// errAmbiguousSourceRead indicates a source-stream read returned no message
// and no error. A genuine orphan always surfaces as a not-found error, so an
// empty result without one is ambiguous.
var errAmbiguousSourceRead = errors.New("ambiguous source stream read: no message and no error")

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

// Option configures a queue operation on failed tasks.
// Pass options via functional options (e.g., WithCluster("foo")). Each
// operation uses only the subset of fields relevant to it.
type Option func(*optionConfig)

type optionConfig struct {
	cluster string
	wals    []string
}

// WithCluster restricts the operation to failed tasks whose original task
// belongs to the given cluster.
func WithCluster(name string) Option {
	return func(c *optionConfig) {
		c.cluster = name
	}
}

// WithWALs restricts the operation to failed tasks for the given WAL file
// names.
func WithWALs(wals ...string) Option {
	return func(c *optionConfig) {
		c.wals = wals
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
func (m *StreamManager) ListFailedWALTasks(ctx context.Context, opts ...Option) ([]FailedTask[WALTask], error) {
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
	opts ...Option,
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

// RetryFailedWALTasks re-enqueues failed WAL tasks from the dead-letter queue.
func (m *StreamManager) RetryFailedWALTasks(
	ctx context.Context,
	opts ...Option,
) error {
	var cfg optionConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	var listOpts []Option
	if cfg.cluster != "" {
		listOpts = append(listOpts, WithCluster(cfg.cluster))
	}

	failedTasks, err := m.ListFailedWALTasks(ctx, listOpts...)
	if err != nil {
		return fmt.Errorf("while listing failed WAL tasks: %w", err)
	}

	if len(cfg.wals) > 0 {
		failedTasks = slices.DeleteFunc(failedTasks, func(task FailedTask[WALTask]) bool {
			return !slices.Contains(cfg.wals, task.Task.WALName)
		})
	}

	return m.reenqueueWALTasks(ctx, failedTasks)
}

// reenqueueWALTasks re-publishes the given failed WAL tasks onto the work queue
// carrying the DLQ retry origin marker, skipping duplicate tasks.
func (m *StreamManager) reenqueueWALTasks(ctx context.Context, tasks []FailedTask[WALTask]) error {
	attempted := make(map[WALTask]struct{}, len(tasks))
	for _, task := range tasks {
		if _, ok := attempted[task.Task]; ok {
			continue
		}
		if err := m.notifyMessage(
			ctx,
			walSubject(task.Task.Cluster()),
			task.Task,
			nats.Header{
				TaskOriginHeaderKey: []string{TaskOriginDLQRetry},
			},
		); err != nil {
			return fmt.Errorf("while retrying failed WAL task for sequence %d: %w", task.Sequence, err)
		}
		attempted[task.Task] = struct{}{}
	}

	return nil
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

func (m *StreamManager) purgeWALDLQEntries(ctx context.Context, clusterName, walName string) error {
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

	failed, err := listFailedTasks[WALTask](ctx, dlqStream, sourceStream, WithCluster(clusterName))
	if err != nil {
		return err
	}

	var errs []error
	for _, task := range failed {
		if task.Task.WALName != walName {
			continue
		}
		if err := m.purgeDLQEntryBySequence(dlqStream, sourceStream, task.Sequence); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
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

// notifyMessage is called to send a message on the queue.
func (m *StreamManager) notifyMessage(ctx context.Context, subject string, task any, headers nats.Header) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Sending message", "subject", subject, "task", task)

	js, err := jetstream.New(m.mgr.NatsConn())
	if err != nil {
		return fmt.Errorf("while creating JetStream instance: %w", err)
	}

	rawContent, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("while marshalling task to JSON: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    rawContent,
		Header:  headers,
	}

	_, err = js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("while pushing message to the queue: %w", err)
	}

	return nil
}

func listFailedTasks[T clusterTask](
	ctx context.Context,
	dlqStream, taskStream *jsm.Stream,
	opts ...Option,
) ([]FailedTask[T], error) {
	var cfg optionConfig
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

// dlqAdvisory is a DLQ entry collected from the pager before its original
// stream message is resolved.
type dlqAdvisory struct {
	// dlqSequence is the DLQ stream sequence of the advisory itself.
	dlqSequence uint64
	// timestamp is when the advisory was stored on the DLQ stream.
	timestamp time.Time
	// sourceSeq is the sequence of the failed message on the source stream.
	sourceSeq uint64
}

func listPager[T clusterTask](ctx context.Context,
	pgr *jsm.StreamPager,
	readMessage func(seq uint64) (*api.StoredMsg, error),
	expected uint64,
	cfg optionConfig,
) ([]FailedTask[T], error) {
	// The DLQ pager and the source-stream reads share the same NATS
	// connection, and the pager's reply inbox shares the connection's
	// request-mux inbox prefix. Issuing a source-stream read while the pager
	// still has batch replies in flight can make the read pick up a stray
	// pager reply, surfacing as a spurious empty message. So we drain the
	// pager fully first and only then resolve the source messages.
	advisories, err := drainDLQPager(ctx, pgr, expected)
	if err != nil {
		return nil, err
	}

	tasks := make([]FailedTask[T], 0, len(advisories))
	for _, advisory := range advisories {
		msg, err := readMessage(advisory.sourceSeq)
		switch {
		case jsm.IsNatsError(err, uint16(server.JSNoMessageFoundErr)):
			// The DLQ advisory outlives the original message: once a failed
			// task is retried or the source stream is purged, the advisory
			// remains but its source message is gone. A genuine orphan always
			// surfaces as this not-found error, so skip it instead of failing
			// the whole listing.
			log.FromContext(ctx).Info(
				"Skipping orphaned DLQ entry: original stream message not found",
				"streamSeq", advisory.sourceSeq,
			)

			continue
		case err != nil:
			return nil, fmt.Errorf("failed to read original stream message: %w", err)
		case msg == nil:
			return nil, fmt.Errorf("%w: streamSeq %d", errAmbiguousSourceRead, advisory.sourceSeq)
		}

		var task T
		if err := json.Unmarshal(msg.Data, &task); err != nil {
			return nil, fmt.Errorf("failed to unmarshal original stream message data: %w", err)
		}

		if cfg.cluster != "" && task.Cluster() != cfg.cluster {
			continue
		}

		tasks = append(tasks, FailedTask[T]{
			Sequence:  advisory.dlqSequence,
			Task:      task,
			Timestamp: advisory.timestamp,
		})
	}

	slices.SortFunc(tasks, func(a, b FailedTask[T]) int {
		return b.Timestamp.Compare(a.Timestamp)
	})

	return tasks, nil
}

// drainDLQPager reads every advisory the pager can deliver. It returns an
// error if it reads fewer entries than expected, because the pager cannot
// distinguish a genuine end-of-stream from a timeout and a short read would
// otherwise produce a silently truncated listing.
func drainDLQPager(ctx context.Context, pgr *jsm.StreamPager, expected uint64) ([]dlqAdvisory, error) {
	advisories := make([]dlqAdvisory, 0, expected)

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

		dlqMetadata, err := jsm.ParseJSMsgMetadata(dlqMsg)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DLQ stream message metadata: %w", err)
		}

		var dlqTask server.JSConsumerDeliveryExceededAdvisory
		if err := json.Unmarshal(dlqMsg.Data, &dlqTask); err != nil {
			return nil, fmt.Errorf("failed to unmarshal DLQ stream message data: %w", err)
		}

		advisories = append(advisories, dlqAdvisory{
			dlqSequence: dlqMetadata.StreamSequence(),
			timestamp:   dlqMetadata.TimeStamp(),
			sourceSeq:   dlqTask.StreamSeq,
		})
	}

	// If we read fewer entries than the stream reported, the pager gave up
	// early (slow/unreachable server) and the listing would be silently
	// truncated, so fail loudly instead.
	if read := uint64(len(advisories)); read < expected {
		return nil, fmt.Errorf("%w: read %d of %d entries", errIncompleteDLQListing, read, expected)
	}

	return advisories, nil
}
