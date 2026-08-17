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
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"time"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

type incoherentRequestError struct {
	expectedValue string
	foundValue    string
	involvedField string
}

func (e *incoherentRequestError) Error() string {
	return fmt.Sprintf(
		"incoherent %s, expected %s found %s",
		e.involvedField,
		e.expectedValue,
		e.foundValue,
	)
}

type walUploadBlockMetadata struct {
	clusterName string
	walFileName string
	segmentSize uint64
	sendToTier2 bool
}

func (m *walUploadBlockMetadata) handleRequest(request *grpc.PutRequest) error {
	if request.GetClusterName() == "" {
		return errEmptyClusterName
	}
	if request.GetWalName() == "" {
		return errEmptyWALName
	}
	if request.GetSegmentSize() == 0 {
		return errEmptySegmentSize
	}

	if m.clusterName == "" {
		m.clusterName = request.GetClusterName()
	} else if m.clusterName != request.GetClusterName() {
		return &incoherentRequestError{
			involvedField: "cluster name",
			expectedValue: m.clusterName,
			foundValue:    request.GetClusterName(),
		}
	}

	if m.walFileName == "" {
		m.walFileName = request.GetWalName()
	} else if m.walFileName != request.GetWalName() {
		return &incoherentRequestError{
			involvedField: "wal name",
			expectedValue: m.walFileName,
			foundValue:    request.GetWalName(),
		}
	}

	if m.segmentSize == 0 {
		m.segmentSize = request.GetSegmentSize()
	} else if m.segmentSize != request.GetSegmentSize() {
		return &incoherentRequestError{
			involvedField: "wal segment size",
			expectedValue: strconv.FormatUint(m.segmentSize, 10),
			foundValue:    strconv.FormatUint(request.GetSegmentSize(), 10),
		}
	}

	m.sendToTier2 = request.GetSendToTier2()

	return nil
}

// putHandler carries the state of a single WAL Put streaming call.
type putHandler struct {
	impl      *Implementation
	req       grpc.WAL_PutServer
	logger    log.Logger
	startTime time.Time

	blockMeta   walUploadBlockMetadata
	walBuffer   *repository.Writer
	writtenSize uint64

	// spanEnriched records whether the cluster and WAL name have already been
	// attached to the RPC span, so we set them only once per Put call.
	spanEnriched bool
}

// Put uploads a new WAL to the data store.
func (w *Implementation) Put(req grpc.WAL_PutServer) error {
	if w.isReadOnly {
		return status.Error(grpccodes.FailedPrecondition, errReadOnly.Error())
	}

	h := &putHandler{
		impl:      w,
		req:       req,
		logger:    log.FromContext(req.Context()),
		startTime: time.Now(),
	}

	return h.run(req.Context())
}

// run consumes the stream of WAL blocks and finalizes the upload. A receive
// error (including io.EOF) stops the loop and the upload is finalized with
// whatever has been written so far.
func (h *putHandler) run(ctx context.Context) error {
	for {
		request, err := h.receiveBlock()
		if err != nil {
			break
		}

		if err := h.processBlock(ctx, request); err != nil {
			return err
		}
	}

	return h.finalize(ctx)
}

// receiveBlock reads the next WAL block from the stream. It returns an error
// (including io.EOF) once the stream is exhausted or fails. Per-block send
// latency is measured on the sender side (the client's `send` stage), not here:
// this call's duration is dominated by waiting for the client to produce the
// next block, which is gated by PostgreSQL's WAL generation rate.
func (h *putHandler) receiveBlock() (*grpc.PutRequest, error) {
	request, err := h.req.Recv()
	if err != nil {
		if !errors.Is(err, io.EOF) {
			h.logger.Warning(
				"Error while reading WAL block",
				"clusterName", request.GetClusterName(),
				"walName", request.GetWalName(),
				"err", err,
			)
		}

		return nil, err
	}

	return request, nil
}

// processBlock validates a received block, writes it to the WAL buffer and
// updates the latest-written metrics.
func (h *putHandler) processBlock(ctx context.Context, request *grpc.PutRequest) error {
	if err := h.validateRequest(request); err != nil {
		return err
	}

	h.logger.Debug(
		"Received WAL block",
		"clusterName", request.GetClusterName(),
		"walName", request.GetWalName(),
		"blockLen", len(request.GetWalBlock()),
	)

	if err := h.blockMeta.handleRequest(request); err != nil {
		h.logger.Error(
			err,
			"Incoherent WAL Block received",
			"clusterName", request.GetClusterName(),
			"walName", request.GetWalName(),
		)

		return status.Errorf(grpccodes.InvalidArgument, "%s", err.Error())
	}

	h.enrichSpan(ctx)
	h.recordLatestWrittenTimeline(ctx)

	if err := h.openWriter(request); err != nil {
		return err
	}

	if err := h.writeBlock(ctx, request); err != nil {
		return err
	}

	h.writtenSize += uint64(len(request.GetWalBlock()))
	h.recordLatestWrittenLSN(ctx)

	// The block has been fsynced by writeBlock, so acknowledge the newly
	// durable position back to the client. This lets a client that requires
	// durable acknowledgements advance the flush position it reports to
	// PostgreSQL at a per-block granularity instead of once per whole segment.
	return h.sendAck()
}

// sendAck reports the cumulative number of durably persisted bytes to the
// client on the Put stream.
func (h *putHandler) sendAck() error {
	if err := h.req.Send(&grpc.PutResult{WrittenSize: h.writtenSize}); err != nil {
		h.logger.Warning(
			"Error while sending WAL Put acknowledgement",
			"writtenSize", h.writtenSize,
			"walFileName", h.blockMeta.walFileName,
			"clusterName", h.blockMeta.clusterName,
			"err", err,
		)

		return status.Errorf(grpccodes.Internal, "error while sending acknowledgement: %v", err.Error())
	}

	return nil
}

// validateRequest checks the cluster name and WAL name of a received block.
func (h *putHandler) validateRequest(request *grpc.PutRequest) error {
	if err := repository.ValidatePathComponent(request.GetClusterName()); err != nil {
		h.logger.Warning("Wrong cluster name used in WAL Put", "clusterName", request.GetClusterName())
		return status.Errorf(grpccodes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := repository.ValidatePathComponent(request.GetWalName()); err != nil {
		h.logger.Warning("Wrong WAL name used in WAL Put", "walName", request.GetWalName())
		return status.Errorf(grpccodes.InvalidArgument, "invalid WAL name: %v", err.Error())
	}

	if err := repository.ValidateWalFileName(request.GetWalName()); err != nil {
		return status.Errorf(grpccodes.InvalidArgument, "invalid WAL name: %q", request.GetWalName())
	}

	return nil
}

// openWriter lazily creates the WAL buffer writer on the first received block.
func (h *putHandler) openWriter(request *grpc.PutRequest) error {
	if h.walBuffer != nil {
		return nil
	}

	walBuffer, err := h.impl.conn.NewWriter(
		repository.WriterOptions{
			ClusterName: h.blockMeta.clusterName,
			WALName:     h.blockMeta.walFileName,
			SegmentSize: h.blockMeta.segmentSize,
			Metrics:     h.impl.metrics,
		},
	)
	if err != nil {
		h.logger.Error(
			err,
			"Cannot open new WAL file",
			"clusterName", request.GetClusterName(),
			"walName", request.GetWalName(),
		)

		return status.Errorf(grpccodes.Internal, "error while opening new WAL: %v", err.Error())
	}

	h.walBuffer = walBuffer

	return nil
}

// writeBlock writes and flushes a single WAL block to the buffer.
func (h *putHandler) writeBlock(ctx context.Context, request *grpc.PutRequest) error {
	if err := h.walBuffer.WriteBlock(ctx, request.GetWalBlock()); err != nil {
		h.logger.Error(
			err,
			"Error while writing WAL data",
			"clusterName", request.GetClusterName(),
			"walName", request.GetWalName(),
		)

		return status.Errorf(grpccodes.Internal, "error while writing WAL: %v", err.Error())
	}

	flushStart := time.Now()
	err := h.walBuffer.Flush()
	flushDuration := time.Since(flushStart)
	if err != nil {
		h.logger.Error(
			err,
			"Error while flushing WAL data",
			"clusterName", request.GetClusterName(),
			"walName", request.GetWalName(),
		)

		h.impl.metrics.RecordBlockStage(ctx, request.GetClusterName(), opentelemetry.PathPut,
			opentelemetry.StageFlush, flushDuration, opentelemetry.OutcomeFailure)

		return status.Errorf(grpccodes.Internal, "error while flushing WAL: %v", err.Error())
	}
	h.impl.metrics.RecordBlockStage(ctx, request.GetClusterName(), opentelemetry.PathPut,
		opentelemetry.StageFlush, flushDuration, opentelemetry.OutcomeSuccess)

	return nil
}

// enrichSpan attaches the cluster and WAL name to the RPC span created by the
// otelgrpc stats handler. The names are only known once the first block has
// been received, so the span cannot carry them at creation time; we set them
// once, on the first block, to match the attributes of the tier-2 upload span.
func (h *putHandler) enrichSpan(ctx context.Context) {
	if h.spanEnriched {
		return
	}

	trace.SpanFromContext(ctx).SetAttributes(
		opentelemetry.AttributeKeyClusterName.Of(h.blockMeta.clusterName),
		opentelemetry.AttributeKeyWalName.Of(h.blockMeta.walFileName),
	)
	h.spanEnriched = true
}

// recordLatestWrittenTimeline updates the latest written timeline metric for
// real WAL segments. Non-segment files such as .history, .backup and .partial
// are not parseable as segments and are skipped to avoid spurious parse errors.
func (h *putHandler) recordLatestWrittenTimeline(ctx context.Context) {
	timeline, err := timelineFromWALName(h.blockMeta.walFileName)
	if errors.Is(err, errNotWALSegment) {
		return
	}
	if err != nil {
		h.logger.Error(err, "Could not parse timeline for latest written timeline metric",
			"walName", h.blockMeta.walFileName)
		return
	}

	h.impl.metrics.LatestWrittenTimeline.Record(
		ctx,
		timeline,
		metric.WithAttributeSet(
			h.impl.metrics.AttributeSet(opentelemetry.AttributeKeyClusterName.Of(h.blockMeta.clusterName)),
		),
	)
}

// recordLatestWrittenLSN updates the latest written LSN metric for real WAL
// segments. Non-segment files are skipped, as for the timeline metric.
func (h *putHandler) recordLatestWrittenLSN(ctx context.Context) {
	startPos, err := lsnStartFromWALName(h.blockMeta.walFileName, h.blockMeta.segmentSize)
	if errors.Is(err, errNotWALSegment) {
		return
	}
	if err != nil {
		h.logger.Error(err, "Could not compute start LSN for latest written LSN metric",
			"walName", h.blockMeta.walFileName)
		return
	}

	h.impl.metrics.LatestWrittenLSN.Record(
		ctx,
		int64(startPos+h.writtenSize), //nolint:gosec
		metric.WithAttributeSet(
			h.impl.metrics.AttributeSet(opentelemetry.AttributeKeyClusterName.Of(h.blockMeta.clusterName)),
		),
	)
}

// timelineFromWALName returns the timeline ID of a WAL segment file. It returns
// errNotWALSegment for non-segment files (.history, .backup, .partial), which
// must not drive the latest_written_* metrics.
func timelineFromWALName(walFileName string) (int64, error) {
	if !postgres.IsWALFile(walFileName) {
		return 0, errNotWALSegment
	}

	segment, err := postgres.SegmentFromName(walFileName)
	if err != nil {
		return 0, err
	}

	return int64(segment.Tli), nil
}

// lsnStartFromWALName returns the start LSN position of a WAL segment file. It
// returns errNotWALSegment for non-segment files, as for timelineFromWALName.
func lsnStartFromWALName(walFileName string, segmentSize uint64) (uint64, error) {
	if !postgres.IsWALFile(walFileName) {
		return 0, errNotWALSegment
	}

	startLSN, err := types.LSNStartFromWALName(walFileName, segmentSize)
	if err != nil {
		return 0, err
	}

	startPos, err := startLSN.Parse()
	if err != nil {
		return 0, err
	}

	return startPos, nil
}

// finalize closes the WAL buffer, reports the result to the client and enqueues
// tier-2 work when requested.
func (h *putHandler) finalize(ctx context.Context) error {
	if h.walBuffer == nil {
		return h.closeEmpty()
	}

	if err := h.closeBuffer(ctx); err != nil {
		return err
	}

	// Send a terminal acknowledgement carrying the final durable size. This
	// mirrors the last per-block acknowledgement and guarantees the client
	// observes the complete size before the stream is closed.
	if err := h.sendAck(); err != nil {
		return err
	}

	return h.notifyTier2(ctx)
}

// closeEmpty reports an empty result when no WAL block was ever received.
func (h *putHandler) closeEmpty() error {
	if err := h.sendAck(); err != nil {
		h.logger.Error(err, "Error while closing empty WAL file")

		return err
	}

	return nil
}

// closeBuffer closes the WAL buffer, distinguishing between a partial and a
// complete WAL segment.
func (h *putHandler) closeBuffer(ctx context.Context) error {
	if !h.isCompleted() {
		return h.closePartial()
	}

	return h.closeComplete(ctx)
}

// isCompleted returns true if the WAL segment has been fully received.
func (h *putHandler) isCompleted() bool {
	return h.writtenSize == h.blockMeta.segmentSize && h.writtenSize != 0
}

// closePartial closes a partially received WAL file.
func (h *putHandler) closePartial() error {
	if err := h.walBuffer.Close(); err != nil {
		h.logger.Warning(
			"Error while closing partial WAL file",
			"writtenSize", h.writtenSize,
			"walFileName", h.blockMeta.walFileName,
			"clusterName", h.blockMeta.clusterName,
			"err", err,
		)

		return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
	}

	h.logger.Info(
		"Received partial WAL file",
		"writtenSize", h.writtenSize,
		"segmentSize", h.blockMeta.segmentSize,
		"walFileName", h.blockMeta.walFileName,
		"clusterName", h.blockMeta.clusterName,
		"elapsedTime", time.Since(h.startTime),
	)

	return nil
}

// closeComplete closes a fully received WAL file and records its metrics.
func (h *putHandler) closeComplete(ctx context.Context) error {
	if err := h.walBuffer.CloseMarkDone(); err != nil {
		h.logger.Warning(
			"Error while closing completed WAL file",
			"writtenSize", h.writtenSize,
			"walFileName", h.blockMeta.walFileName,
			"clusterName", h.blockMeta.clusterName,
			"err", err,
		)

		return status.Errorf(grpccodes.Internal, "error while closing (done) WAL: %v", err.Error())
	}

	clusterAttr := metric.WithAttributeSet(
		h.impl.metrics.AttributeSet(opentelemetry.AttributeKeyClusterName.Of(h.blockMeta.clusterName)),
	)

	h.impl.metrics.WalWritten.Add(ctx, 1, clusterAttr)
	h.impl.metrics.LatestWrittenTime.Record(ctx, time.Now().Unix(), clusterAttr)

	h.logger.Info(
		"Received completed WAL file",
		"writtenSize", h.writtenSize,
		"segmentSize", h.blockMeta.segmentSize,
		"walFileName", h.blockMeta.walFileName,
		"clusterName", h.blockMeta.clusterName,
		"elapsedTime", time.Since(h.startTime),
	)

	return nil
}

// notifyTier2 enqueues the WAL for tier-2 processing when requested.
func (h *putHandler) notifyTier2(ctx context.Context) error {
	if !h.blockMeta.sendToTier2 {
		return nil
	}

	if h.impl.queue == nil {
		return status.Errorf(
			grpccodes.Internal,
			"queue service is uninitialized",
		)
	}

	if err := h.impl.queue.NotifyWALReceived(ctx, &queue.WALTask{
		ClusterName: h.blockMeta.clusterName,
		WALName:     path.Base(h.walBuffer.WALFilePath()),
	}); err != nil {
		return status.Errorf(
			grpccodes.Internal,
			"error while putting WAL message into the queue, please retry: %v",
			err.Error(),
		)
	}

	return nil
}
