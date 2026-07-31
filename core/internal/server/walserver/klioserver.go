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

package walserver

import (
	"go.opentelemetry.io/otel/attribute"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// Implementation is the implementation of the WAL server.
type Implementation struct {
	grpc.UnimplementedWALServer

	conn       *repository.Connection
	metrics    *repository.Metrics
	isReadOnly bool
	queue      *queue.Conn
}

// Options is the structure describing the behavior of
// a WAL server.
type Options struct {
	// Connection is the repository to be used with this WAL server
	Connection *repository.Connection

	// ReadOnly is true when the repository should deny any write attempt
	ReadOnly bool

	// Queue is the connection to the Queue
	Queue *queue.Conn

	// Tier is the storage tier this WAL server serves, used to tag its metrics
	// (a tier-1 server reads/writes local disk; a tier-2 server serves WAL from
	// remote object storage).
	Tier opentelemetry.Tier
}

// New creates a new WAL server implementation.
func New(
	opts Options,
) *Implementation {
	return &Implementation{
		conn:       opts.Connection,
		isReadOnly: opts.ReadOnly,
		metrics: &repository.Metrics{
			WalWrittenBytes:       opentelemetry.ServerWal.WalWrittenBytes,
			WalWritten:            opentelemetry.ServerWal.WalWritten,
			LatestWrittenTime:     opentelemetry.ServerWal.LatestWrittenTime,
			LatestWrittenLSN:      opentelemetry.ServerWal.LatestWrittenLSN,
			LatestWrittenTimeline: opentelemetry.ServerWal.LatestWrittenTimeline,
			BlockDuration:         opentelemetry.ServerWal.BlockDuration,
			Attributes:            []attribute.KeyValue{opts.Tier.Attribute()},
		},
		queue: opts.Queue,
	}
}
