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

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"

	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// DLQWatcher counts the tasks moved to the dead-letter queue, feeding the
// klio.server.queue.dlq.captured metric.
type DLQWatcher struct {
	QueueURL string
	Watcher  *queue.DLQWatcher
}

// NewDLQWatcher creates a DLQWatcher counting the dead-letter queue entries
// stored from now on. Create it before the queue server starts.
func NewDLQWatcher(queueURL string) *DLQWatcher {
	return &DLQWatcher{
		QueueURL: queueURL,
		Watcher:  queue.NewDLQWatcher(time.Now()),
	}
}

// Serve follows the dead-letter queue streams until ctx is canceled.
func (s *DLQWatcher) Serve(ctx context.Context) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithName("dlq-watcher"))

	natsConnection, err := nats.Connect(
		s.QueueURL,
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(1*time.Second),
	)
	if err != nil {
		return fmt.Errorf("error while connecting to the NATS server: %w", err)
	}
	defer natsConnection.Close()

	queueConnection, err := queue.New(ctx, natsConnection)
	if err != nil {
		return fmt.Errorf("error while configuring NATS server: %w", err)
	}

	return s.Watcher.Run(ctx, queueConnection)
}

func (s *DLQWatcher) String() string {
	return "dlq-watcher"
}
