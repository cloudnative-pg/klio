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

package consumer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/wal"
)

// tier2BufferSize is the write buffer size for tier2 WAL streaming.
const tier2BufferSize = 4 * 1024 * 1024

// WAL represents a WAL consumer.
type WAL struct {
	metrics *repository.Metrics
	opts    *WALOptions
}

// WALOptions are the configuration of the WAL consumer.
type WALOptions struct {
	// The queue to be used
	Queue *queue.Conn

	// A connection to tier 1
	Tier1 *repository.Connection

	// A connection to tier 2
	Tier2 *repository.Connection
}

// NewWAL creates a new WAL consumer.
func NewWAL(opts *WALOptions) *WAL {
	return &WAL{
		metrics: &repository.Metrics{
			WalWrittenBytes:       opentelemetry.ServerWal.WalWrittenBytes,
			WalWritten:            opentelemetry.ServerWal.WalWritten,
			LatestWrittenTime:     opentelemetry.ServerWal.LatestWrittenTime,
			LatestWrittenLSN:      opentelemetry.ServerWal.LatestWrittenLSN,
			LatestWrittenTimeline: opentelemetry.ServerWal.LatestWrittenTimeline,
			Attributes:            []attribute.KeyValue{opentelemetry.Tier2.Attribute()},
		},
		opts: opts,
	}
}

// Run starts the consumer until the context is canceled or the
// SIGINT signal arrives.
func (d *WAL) Run(ctx context.Context) error {
	consumerCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	return d.opts.Queue.ConsumeWALReceivedMessages(consumerCtx, d.walHandler)
}

//nolint:cyclop
func (d *WAL) walHandler(ctx context.Context, task *queue.WALTask) (returnErr error) {
	logger := log.FromContext(ctx).WithValues("task", task)
	logger.Info("Archiving WAL file")

	ctx, span := tracer.Start(ctx, opentelemetry.Tier2UploadSpan,
		trace.WithAttributes(
			opentelemetry.AttributeKeyClusterName.Of(task.ClusterName),
			opentelemetry.AttributeKeyWalName.Of(task.WALName),
		))
	defer span.End()
	defer func() {
		if returnErr != nil {
			span.RecordError(returnErr)
			span.SetStatus(codes.Error, "tier-2 upload failed")
		}
	}()

	startTime := time.Now()

	// Record the per-file tier-2 upload duration
	defer func() {
		opentelemetry.RecordDuration(ctx, opentelemetry.ServerWal.UploadDuration, time.Since(startTime), returnErr,
			opentelemetry.Tier2.Attribute(), opentelemetry.AttributeKeyClusterName.Of(task.ClusterName))
	}()

	reader, err := repository.NewReader(d.opts.Tier1,
		task.ClusterName,
		task.WALName,
		d.metrics,
	)
	if err != nil {
		return fmt.Errorf("while creating a new WAL reader: %w", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Info("Error while closing the WAL file from Tier 1")
		}
	}()

	writer, err := d.opts.Tier2.NewDirectWriter(
		repository.WriterOptions{
			ClusterName: task.ClusterName,
			WALName:     task.WALName,
			SegmentSize: reader.GetFileLength(),
			Metrics:     d.metrics,
			BufferSize:  tier2BufferSize,
		},
	)
	if err != nil {
		return fmt.Errorf("while creating a new WAL writer for Tier 2: %w", err)
	}

	// writtenSize accumulates the raw WAL bytes archived to tier-2. For a
	// complete segment it equals the segment size; for a .partial segment it is
	// smaller, and drives the latest-written-LSN metric accordingly.
	var writtenSize uint64
	defer func() {
		closeErr := writer.Close()
		if closeErr != nil {
			logger.Error(closeErr, "Error while closing the WAL file from Tier 2")
			if returnErr == nil {
				returnErr = fmt.Errorf("while closing Tier 2 WAL: %w", closeErr)
			}

			return
		}

		d.recordArchivedWALMetrics(ctx, task, reader.GetFileLength(), writtenSize)
	}()

	for {
		block, readError := reader.ReadBlock(ctx)
		if readError != nil && !errors.Is(readError, io.EOF) {
			return fmt.Errorf("error while reading WAL: %v", readError.Error())
		}

		writeError := writer.WriteBlock(ctx, block)
		if writeError != nil {
			return fmt.Errorf("error while writing WAL block: %v", writeError.Error())
		}
		writtenSize += uint64(len(block))

		if errors.Is(readError, io.EOF) {
			break
		}
	}

	return nil
}

// recordArchivedWALMetrics emits the per-WAL gauges and counters that
// describe a successfully archived WAL file on Tier 2. segmentSize is the full
// segment size, while writtenSize is the number of bytes actually archived,
// which is smaller than segmentSize for a .partial segment.
func (d *WAL) recordArchivedWALMetrics(ctx context.Context, task *queue.WALTask, segmentSize, writtenSize uint64) {
	logger := log.FromContext(ctx).WithValues("task", task)
	clusterAttr := metric.WithAttributeSet(
		d.metrics.AttributeSet(opentelemetry.AttributeKeyClusterName.Of(task.ClusterName)),
	)

	d.metrics.WalWritten.Add(ctx, 1, clusterAttr)
	d.metrics.LatestWrittenTime.Record(ctx, time.Now().Unix(), clusterAttr)

	// Record LSN and timeline only for complete WAL segments and their .partial
	// variants (such as one left behind by a failover). History and
	// backup-label files carry neither.
	if !wal.IsWALSegmentOrPartial(task.WALName) {
		return
	}

	// The segment parsers below reject the .partial suffix, so recover the bare
	// segment name for them.
	walName := wal.TrimPartialSuffix(task.WALName)

	// Report the LSN of the bytes actually written so a .partial segment does
	// not overstate the archived LSN. An empty segment has no LSN to report.
	if writtenSize > 0 {
		if lsn, err := latestWrittenLSN(walName, segmentSize, writtenSize); err == nil {
			d.metrics.LatestWrittenLSN.Record(ctx, int64(lsn), clusterAttr) //nolint:gosec
		} else {
			logger.Error(err, "Could not compute latest written LSN metric", "walName", walName)
		}
	}

	if segment, err := postgres.SegmentFromName(walName); err == nil {
		d.metrics.LatestWrittenTimeline.Record(ctx, int64(segment.Tli), clusterAttr)
	} else {
		logger.Error(err, "Could not parse timeline for latest written timeline metric",
			"walName", walName)
	}
}

// latestWrittenLSN returns the byte offset of the last LSN durably archived for
// a WAL segment named walName into which writtenSize bytes have been written.
// segmentSize is the full segment size, needed to locate the segment's start
// LSN; for a .partial segment writtenSize is smaller than segmentSize, so the
// returned LSN reflects only the bytes actually archived.
func latestWrittenLSN(walName string, segmentSize, writtenSize uint64) (uint64, error) {
	startLSN, err := types.LSNStartFromWALName(walName, segmentSize)
	if err != nil {
		return 0, fmt.Errorf("while computing start LSN for %s: %w", walName, err)
	}

	startPos, err := startLSN.Parse()
	if err != nil {
		return 0, fmt.Errorf("while parsing start LSN for %s: %w", walName, err)
	}

	return startPos + writtenSize - 1, nil
}
