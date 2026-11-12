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

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/cloudnative-pg/klio/core/internal/wal"
)

// WALTask is the structure that is sent on NATS Stream when
// a new WAL has been received.
type WALTask struct {
	// ClusterName is the name of the cluster
	ClusterName string `json:"clusterName"`

	// WALName if the name of the WAL
	WALName string `json:"walName"`
}

// Cluster returns the name of the cluster associated with this task.
func (t WALTask) Cluster() string {
	return t.ClusterName
}

// NotifyWALReceived is called to notify the consumers that a new WAL
// is available in the Klio repository.
func (q *Conn) NotifyWALReceived(ctx context.Context, task *WALTask) error {
	return q.notifyMessage(ctx, walSubject(task.ClusterName), task, nil)
}

// WALTaskHandler is called for every WAL task message that should be handled.
// If succeeds, the message is not retried.
type WALTaskHandler func(ctx context.Context, t *WALTask) error

// ConsumeWALReceivedMessages starts consuming the WAL received messages and ends
// when the context is canceled.
func (q *Conn) ConsumeWALReceivedMessages(ctx context.Context, handler WALTaskHandler) error {
	logger := log.FromContext(ctx).WithName("wal-consumer")

	// purged is the stream sequence of the last retried message whose
	// dead-letter queue entries were purged.
	var purged uint64

	wrapped := func(ctx context.Context, t *WALTask, headers nats.Header, streamSeq uint64) error {
		isRetried := isDLQRetry(headers)
		if isRetried {
			logger.Info(
				"Retrying WAL task re-enqueued from the dead-letter queue",
				"cluster", t.ClusterName, "wal", t.WALName,
			)
		}

		err := handler(ctx, t)

		// Once a retried task has run, its earlier dead-letter queue entries
		// are redundant whatever the outcome: it either succeeded, or it is
		// retried and, if it keeps failing, dead-lettered again with a fresh
		// entry. Discarding them also releases their original from the WAL
		// stream. The task's cluster name is never empty: the WAL server
		// rejects empty cluster names before enqueuing the task, and
		// purgeWALDLQEntries needs it to select the entries to discard.
		//
		// A failing task is redelivered several times, and each purge lists
		// the whole dead-letter queue, so the purge is skipped when the
		// previous run already did it for the same message. Messages are
		// handled one at a time (MaxAckPending is 1), so the redeliveries of
		// a message are consecutive. The message is told by its stream
		// sequence, not by its task: a task retried again is a new message,
		// and the entry its previous retry left must be purged too.
		if isRetried && (err == nil || streamSeq != purged) {
			if purgeErr := q.purgeWALDLQEntries(ctx, t.ClusterName, t.WALName); purgeErr != nil {
				logger.Error(
					purgeErr,
					"Failed to purge WAL dead-letter queue entries of a retried task",
					"cluster", t.ClusterName, "wal", t.WALName,
				)
			} else {
				purged = streamSeq
			}
		}

		return err
	}

	return internalConsumeMessages(ctx, wrapped, q.walConsumer)
}

// OldestPendingWAL returns the earliest WAL segment (without .partial) that
// clusterName still has in the WAL stream, or "" if none. These WALs have not
// reached tier2 yet, so tier1 must keep them. Every task is checked because
// DLQ retries are re-published at the end of the stream with an older WAL.
//
// This issues one request per pending message, for every cluster and sweep,
// which is fine up to tens of thousands of pending WALs. If a sweep gets
// slow, put the WAL name in the subject (klio.wal.<cluster>.<wal>) and read
// the pending names from one stream Info call with a subject filter.
func (m *StreamManager) OldestPendingWAL(ctx context.Context, clusterName string) (string, error) {
	// A missing stream is an error too: reporting no pending WAL would let
	// retention remove WALs that may still have to be relayed.
	stream, err := m.js.Stream(ctx, klioWalStreamName)
	if err != nil {
		return "", fmt.Errorf("while loading the WAL stream: %w", err)
	}

	subject := walSubject(clusterName)

	var oldest string
	for seq := uint64(1); ; {
		msg, err := stream.GetMsg(ctx, seq, jetstream.WithGetMsgSubject(subject))
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return oldest, nil
		}
		if err != nil {
			return "", fmt.Errorf("while reading the WAL stream at sequence %d: %w", seq, err)
		}

		var task WALTask
		if err := json.Unmarshal(msg.Data, &task); err != nil {
			return "", fmt.Errorf("while unmarshalling the WAL task at sequence %d: %w", msg.Sequence, err)
		}

		oldest = earlierWALSegment(oldest, task.WALName)
		seq = msg.Sequence + 1
	}
}

// earlierWALSegment returns whichever of current and walName names the
// earlier WAL segment, comparing walName without its .partial suffix.
// walName is ignored when it is not a segment (e.g. a .history file), since
// WAL retention only ever removes segments. An empty current means none yet.
func earlierWALSegment(current, walName string) string {
	if !wal.IsWALSegmentOrPartial(walName) {
		return current
	}

	segment := wal.TrimPartialSuffix(walName)
	if current == "" || segment < current {
		return segment
	}

	return current
}
