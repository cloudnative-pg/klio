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
	"github.com/nats-io/jsm.go"
	"github.com/nats-io/nats-server/v2/server"
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
// when the context is canceled. After a successful handler run, the WAL is
// recorded as the latest uploaded WAL for its cluster.
func (q *Conn) ConsumeWALReceivedMessages(ctx context.Context, handler WALTaskHandler) error {
	logger := log.FromContext(ctx).WithName("wal-consumer")

	wrapped := func(ctx context.Context, t *WALTask, headers nats.Header) error {
		isRetried := isDLQRetry(headers)
		if isRetried {
			logger.Info(
				"Retrying WAL task re-enqueued from the dead-letter queue",
				"cluster", t.ClusterName, "wal", t.WALName,
			)
		}

		if err := handler(ctx, t); err != nil {
			// A permanently failed retry will never succeed: release the
			// dead-lettered original too, or it would stay in the WAL stream
			// and hold back tier1 WAL retention (see OldestPendingWAL).
			if isRetried && errors.Is(err, ErrPermanent) {
				if purgeErr := q.purgeWALDLQEntries(ctx, t.ClusterName, t.WALName); purgeErr != nil {
					logger.Error(
						purgeErr,
						"Failed to purge WAL dead-letter queue entries after a permanent failure",
						"cluster", t.ClusterName, "wal", t.WALName,
					)
				}
			}

			return err
		}

		// The task's cluster name is never empty: the WAL server rejects
		// empty cluster names before enqueuing the task, and
		// purgeWALDLQEntries needs it to select the entries to discard.
		if isRetried {
			if err := q.purgeWALDLQEntries(ctx, t.ClusterName, t.WALName); err != nil {
				logger.Error(
					err,
					"Failed to purge WAL dead-letter queue entries after successful retry",
					"cluster", t.ClusterName, "wal", t.WALName,
				)
			}
		}

		if !wal.IsWALSegmentOrPartial(t.WALName) {
			return nil
		}

		// TODO: now that we allow holes in the WAL sequence, the logic of gating the deletion of WALs based on the
		// latest uploaded WAL is not correct anymore. If WAL X and X+1 fail to upload, but X+2 is uploaded, the
		// post-backup clean-up logic may delete WAL X and X+1 even if they have not been uploaded to tier2 yet.
		if err := q.notifyMessage(
			ctx,
			latestUploadedWalSubject(t.ClusterName),
			t,
			nil,
		); err != nil {
			log.FromContext(ctx).Error(
				err,
				"Failed to record latest uploaded WAL, retention safety may degrade",
				"task", t,
			)
		}

		return nil
	}

	return internalConsumeMessages(ctx, wrapped, q.walConsumer)
}

// GetLatestUploadedWAL returns the most recently uploaded WAL file name for
// the given cluster, or empty string if no WAL has been uploaded yet. This is
// used by retention to avoid deleting WALs that have not been transferred to
// tier2.
func (q *Conn) GetLatestUploadedWAL(_ context.Context, clusterName string) (string, error) {
	stream, err := q.loadStreamOrNil(klioLatestUploadedWalStreamName)
	if err != nil {
		return "", err
	}
	if stream == nil {
		return "", nil
	}

	msg, err := stream.ReadLastMessageForSubject(latestUploadedWalSubject(clusterName))
	if err != nil {
		if jsm.IsNatsError(err, uint16(server.JSNoMessageFoundErr)) {
			return "", nil
		}

		return "", fmt.Errorf("while fetching latest uploaded WAL message: %w", err)
	}
	if msg == nil {
		return "", fmt.Errorf("while fetching latest uploaded WAL message: %w", errAmbiguousSourceRead)
	}

	var task WALTask
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		return "", fmt.Errorf("while unmarshalling latest uploaded WAL message: %w", err)
	}

	return task.WALName, nil
}

// OldestPendingWAL returns the earliest WAL segment (without .partial) that
// clusterName still has in the WAL stream, or "" if none. These WALs have not
// reached tier2 yet, so tier1 must keep them. Every task is checked because
// DLQ retries are re-published at the end of the stream with an older WAL.
func (m *StreamManager) OldestPendingWAL(ctx context.Context, clusterName string) (string, error) {
	stream, err := m.js.Stream(ctx, klioWalStreamName)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		return "", nil
	}
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
