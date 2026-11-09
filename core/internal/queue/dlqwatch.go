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
	"sync"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// unknownCluster is the `cluster_name` recorded for a captured entry whose
// original task can no longer be read.
const unknownCluster = "unknown"

// DLQWatcher counts the tasks moved to the dead-letter queue streams. JetStream
// stores the max-deliveries advisories in those streams on its own, so Klio has
// no code path where a task enters the DLQ: the watcher follows the streams
// instead and resolves each entry's cluster from the original task.
//
// A DLQWatcher can be run again after Run returns, for example when its
// supervisor restarts it: it resumes after the last entry it counted, so no
// entry is counted twice.
type DLQWatcher struct {
	since   time.Time
	streams []*dlqStreamWatch
}

// dlqStreamWatch tracks the progress of the watcher on a single DLQ stream.
type dlqStreamWatch struct {
	dlqName string
	// next is the DLQ stream sequence to resume from, or zero to start from
	// the watcher's start time.
	next uint64
}

// NewDLQWatcher creates a DLQWatcher counting the entries stored from since
// onwards. Pass the time the queue server started, so entries captured before
// this process are not counted.
func NewDLQWatcher(since time.Time) *DLQWatcher {
	return &DLQWatcher{
		since: since,
		streams: []*dlqStreamWatch{
			{dlqName: klioDLQWalStreamName},
			{dlqName: klioDLQBackupStreamName},
		},
	}
}

// Run follows the DLQ streams of q until ctx is canceled or a stream cannot be
// followed anymore.
func (w *DLQWatcher) Run(ctx context.Context, q *Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]error, len(w.streams))
	for i, s := range w.streams {
		wg.Go(func() {
			errs[i] = w.watch(ctx, q.js, s)
			// One stream failing stops the other one too, so the caller can
			// restart the whole watcher.
			cancel()
		})
	}
	wg.Wait()

	return errors.Join(errs...)
}

func (w *DLQWatcher) watch(ctx context.Context, js jetstream.JetStream, s *dlqStreamWatch) error {
	cfg := jetstream.OrderedConsumerConfig{
		DeliverPolicy: jetstream.DeliverByStartTimePolicy,
		OptStartTime:  &w.since,
	}
	if s.next > 0 {
		cfg = jetstream.OrderedConsumerConfig{
			DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:   s.next,
		}
	}

	consumer, err := js.OrderedConsumer(ctx, s.dlqName, cfg)
	if err != nil {
		return fmt.Errorf("while creating the ordered consumer on %q: %w", s.dlqName, err)
	}

	msgs, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("while consuming %q: %w", s.dlqName, err)
	}
	stop := context.AfterFunc(ctx, msgs.Stop)
	defer stop()

	for {
		msg, err := msgs.Next()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, jetstream.ErrMsgIteratorClosed) {
				return nil
			}

			return fmt.Errorf("while reading %q: %w", s.dlqName, err)
		}

		meta, err := msg.Metadata()
		if err != nil {
			return fmt.Errorf("while reading the metadata of a %q entry: %w", s.dlqName, err)
		}

		recordDLQEntry(ctx, opentelemetry.ServerQueue.DLQCaptured, s.dlqName, capturedCluster(ctx, js, msg.Data()))
		s.next = meta.Sequence.Stream + 1
	}
}

// capturedCluster returns the cluster of the task referenced by the given
// max-deliveries advisory, or unknownCluster when it cannot be read.
func capturedCluster(ctx context.Context, js jetstream.JetStream, data []byte) string {
	logger := log.FromContext(ctx)

	var advisory server.JSConsumerDeliveryExceededAdvisory
	if err := json.Unmarshal(data, &advisory); err != nil {
		logger.Error(err, "Cannot decode dead-letter queue advisory")

		return unknownCluster
	}

	stream, err := js.Stream(ctx, advisory.Stream)
	if err != nil {
		logger.Info("Cannot open the stream of a dead-letter queue entry",
			"stream", advisory.Stream, "err", err)

		return unknownCluster
	}

	original, err := stream.GetMsg(ctx, advisory.StreamSeq)
	if err != nil {
		logger.Info("Cannot read the original task of a dead-letter queue entry",
			"stream", advisory.Stream, "streamSeq", advisory.StreamSeq, "err", err)

		return unknownCluster
	}

	// WAL and backup tasks share the clusterName field.
	var task struct {
		ClusterName string `json:"clusterName"`
	}
	if err := json.Unmarshal(original.Data, &task); err != nil || task.ClusterName == "" {
		logger.Info("Cannot read the cluster of a dead-letter queue entry",
			"stream", advisory.Stream, "streamSeq", advisory.StreamSeq)

		return unknownCluster
	}

	return task.ClusterName
}
