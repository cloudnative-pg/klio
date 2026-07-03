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
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/jsm.go"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

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
			return err
		}

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

		// TODO: make the latest-uploaded-WAL marker monotonic. Retention treats
		// this record as a high-water mark and won't delete tier1 WALs newer
		// than it. A CLI retry re-injects an older WAL, regressing the marker;
		// it fails safe (retention just gets more conservative and self-heals)
		// but can briefly stall tier1 reclamation. Fix: only advance when
		// t.WALName is lexicographically greater than the stored value.
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

	var task WALTask
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		return "", fmt.Errorf("while unmarshalling latest uploaded WAL message: %w", err)
	}

	return task.WALName, nil
}
