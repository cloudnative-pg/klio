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

package sendwal

import (
	"context"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cloudnative-pg/klio/core/internal/wal"
)

// feedbackSender decouples sending standby status updates to PostgreSQL from
// the rate at which new feedback values arrive and reply requests come in.
type feedbackSender struct {
	feedbackChannel <-chan wal.Feedback
	conn            *pgconn.PgConn
	latest          wal.Feedback

	wake chan struct{}
	done chan struct{}
}

// newFeedbackSender starts the background send loop for conn. Stop must be
// called once the sender is no longer needed, to release the goroutine.
func newFeedbackSender(
	ctx context.Context,
	conn *pgconn.PgConn,
	feedbackChannel <-chan wal.Feedback,
) *feedbackSender {
	s := &feedbackSender{
		wake:            make(chan struct{}, 1),
		done:            make(chan struct{}),
		conn:            conn,
		feedbackChannel: feedbackChannel,
	}

	go s.run(ctx)

	return s
}

// Send queues update to be sent.
func (s *feedbackSender) Send() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stop waits for the background goroutine to exit. It does not by itself
// make that happen: the caller must close feedbackChannel first. Once Stop returns,
// nothing is writing to conn on this sender's behalf. Safe to call more than once.
func (s *feedbackSender) Stop() {
	<-s.done
}

func (s *feedbackSender) run(ctx context.Context) {
	defer close(s.done)

	for {
		select {
		case latest, ok := <-s.feedbackChannel:
			if !ok {
				return
			}

			s.latest = latest
			s.sendLatest(ctx)
		case <-s.wake:
			s.sendLatest(ctx)
		}
	}
}

func (s *feedbackSender) sendLatest(ctx context.Context) {
	msg := pglogrepl.StandbyStatusUpdate{
		WALWritePosition: pglogrepl.LSN(s.latest.WriteLSN),
		WALFlushPosition: pglogrepl.LSN(s.latest.FlushLSN),
		WALApplyPosition: pglogrepl.LSN(s.latest.ReplayLSN),
		ClientTime:       time.Now(),
	}
	if err := pglogrepl.SendStandbyStatusUpdate(ctx, s.conn, msg); err != nil {
		log.FromContext(ctx).Error(err, "Failed to send standby status update, skipping")
	}
}
