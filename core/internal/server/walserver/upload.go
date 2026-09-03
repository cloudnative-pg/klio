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

// setOrVerify records field's first value and rejects any later value that
// disagrees, so a stream can't silently switch identity (cluster, WAL name,
// segment size, start LSN) partway through.
func setOrVerify[T comparable](field *T, incoming T, fieldName string) error {
	var zero T
	if *field == zero {
		*field = incoming
		return nil
	}

	if *field != incoming {
		return &incoherentRequestError{
			involvedField: fieldName,
			expectedValue: fmt.Sprint(*field),
			foundValue:    fmt.Sprint(incoming),
		}
	}

	return nil
}

type walUploadBlockMetadata struct {
	clusterName string
	walFileName string
	segmentSize uint64
	sendToTier2 bool
	walStartLSN uint64
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

	if err := setOrVerify(&m.clusterName, request.GetClusterName(), "cluster name"); err != nil {
		return err
	}
	if err := setOrVerify(&m.walFileName, request.GetWalName(), "wal name"); err != nil {
		return err
	}
	if err := setOrVerify(&m.segmentSize, request.GetSegmentSize(), "wal segment size"); err != nil {
		return err
	}
	if err := setOrVerify(&m.walStartLSN, request.GetWalStartLsn(), "wal start lsn"); err != nil {
		return err
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

	blockMeta walUploadBlockMetadata
	walBuffer *repository.Writer

	writtenBytes uint64
	flushedBytes uint64

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

// run consumes the stream of WAL blocks and finalizes the upload.
//
// Blocks are received on a background goroutine (blockReceiver) and drained
// in batches, so a burst of blocks already queued on the wire is written
// and flushed together - one fsync per batch instead of one per block.
// Progress acks are similarly coalesced by a background goroutine
// (feedbackSenderToKlioClient), so a fast batch of writes doesn't turn into
// a matching burst of synchronous sends back to the client.
//
// A receive error (including io.EOF) stops the loop and the upload is
// finalized with whatever has been written so far - this matches the
// previous behavior of receiveBlock, which never distinguished EOF from
// other read errors for control flow purposes.
//
// Neither receiver nor feedback is stopped via defer: finalize sends the
// closing PutResult directly on h.req, and req.Send is not safe to call
// concurrently with feedback's own background sends, so feedback must be
// stopped before every return. receiver additionally can't always be
// stopped the same way - see the two exit branches below.
func (h *putHandler) run(ctx context.Context) error {
	receiver := newBlockReceiverFromKlioClient(ctx, h.req)
	feedback := newFeedbackSenderToKlioClient(ctx, h.req)

	for {
		batch, recvErr := receiver.Drain(ctx)
		if recvErr != nil {
			// Drain only ever returns an error together with an empty
			// batch (see blockReceiver.Drain), at which point req.Recv has
			// already returned: Stop can only be a fast join here, never a
			// hang.
			receiver.Stop()
			feedback.Stop()

			if !errors.Is(recvErr, io.EOF) {
				h.logger.Warning(
					"Error while reading WAL block",
					"clusterName", h.blockMeta.clusterName,
					"walName", h.blockMeta.walFileName,
					"err", recvErr,
				)
			}

			return h.finalize(ctx)
		}

		if err := h.processBatch(ctx, batch, feedback); err != nil {
			// req.Recv may still be blocked waiting on the client here.
			// Signal only, don't join: returning err below is what
			// eventually unblocks it, once grpc-go cancels h.req.Context()
			// as part of ending the RPC - see blockReceiver.Stop.
			receiver.Cancel()
			feedback.Stop()

			return err
		}
	}
}

// processBatch validates every block in a batch, then writes their payloads
// as a single concatenated write, followed by one flush and one feedback
// update for the whole batch. Concatenating first matters as much as the
// single flush: the WAL writer wraps (compresses/encrypts) and
// length-prefixes each write independently, so writing block-by-block would
// still turn a burst of small blocks into a burst of tiny wrapped chunks in
// the WAL file, even though they'd all share one fsync.
func (h *putHandler) processBatch(
	ctx context.Context,
	batch []*grpc.PutRequest,
	feedback *feedbackSenderToKlioClient,
) error {
	var payload []byte

	for _, request := range batch {
		if err := h.validateBlock(ctx, request); err != nil {
			return err
		}

		payload = append(payload, request.GetWalBlock()...)
	}

	if err := h.writeBlock(ctx, payload); err != nil {
		return err
	}

	h.writtenBytes += uint64(len(payload))
	feedback.SetFeedback(h.writtenBytes+h.blockMeta.walStartLSN, h.flushedBytes+h.blockMeta.walStartLSN)

	if err := h.flushBuffer(ctx); err != nil {
		return err
	}

	h.flushedBytes += uint64(len(payload))
	feedback.SetFeedback(h.writtenBytes+h.blockMeta.walStartLSN, h.flushedBytes+h.blockMeta.walStartLSN)

	h.impl.metrics.LatestWrittenLSN.Record(
		ctx,
		int64(h.flushedBytes+h.blockMeta.walStartLSN), //nolint:gosec
		metric.WithAttributeSet(
			h.impl.metrics.AttributeSet(opentelemetry.AttributeKeyClusterName.Of(h.blockMeta.clusterName)),
		),
	)

	return nil
}

// validateBlock validates a single received block and updates the shared
// metadata. It does not write the block's payload - see processBatch.
func (h *putHandler) validateBlock(ctx context.Context, request *grpc.PutRequest) error {
	if err := h.validateRequest(request); err != nil {
		return err
	}

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

	return h.openWriter(request)
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

// putWriteBufferSize is the write buffer size for the WAL writer opened by
// a Put call. Batches of blocks already share a single WriteBlock call (see
// processBatch), so this mainly bounds the buffer for the rare batch that
// exceeds it, coalescing the underlying writes rather than trickling them
// out block-by-block.
const putWriteBufferSize = 4 * 1024 * 1024

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
			BufferSize:  putWriteBufferSize,
			WALStartLSN: h.blockMeta.walStartLSN,
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

// writeBlock writes a batch's concatenated payload to the buffer, without
// flushing it.
func (h *putHandler) writeBlock(ctx context.Context, data []byte) error {
	if err := h.walBuffer.WriteBlock(ctx, data); err != nil {
		h.logger.Error(
			err,
			"Error while writing WAL data",
			"clusterName", h.blockMeta.clusterName,
			"walName", h.blockMeta.walFileName,
		)

		return status.Errorf(grpccodes.Internal, "error while writing WAL: %v", err.Error())
	}

	return nil
}

// flushBuffer flushes the WAL buffer, covering every block written to it
// since the last flush.
func (h *putHandler) flushBuffer(ctx context.Context) error {
	flushStart := time.Now()
	err := h.walBuffer.Flush()
	flushDuration := time.Since(flushStart)
	if err != nil {
		h.logger.Error(
			err,
			"Error while flushing WAL data",
			"clusterName", h.blockMeta.clusterName,
			"walName", h.blockMeta.walFileName,
		)

		h.impl.metrics.RecordBlockStage(ctx, h.blockMeta.clusterName, opentelemetry.PathPut,
			opentelemetry.StageFlush, flushDuration, opentelemetry.OutcomeFailure)

		return status.Errorf(grpccodes.Internal, "error while flushing WAL: %v", err.Error())
	}
	h.impl.metrics.RecordBlockStage(ctx, h.blockMeta.clusterName, opentelemetry.PathPut,
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

	if err := h.req.Send(&grpc.PutResult{
		FlushLsn: h.flushedBytes,
		WriteLsn: h.writtenBytes,
	}); err != nil {
		h.logger.Warning(
			"Error while sending WAL Put response",
			"flushedLSN", h.flushedBytes,
			"walFileName", h.blockMeta.walFileName,
			"clusterName", h.blockMeta.clusterName,
			"err", err,
		)

		return status.Errorf(grpccodes.Internal, "error while sending response: %v", err.Error())
	}

	return h.notifyTier2(ctx)
}

// closeEmpty reports an empty result when no WAL block was ever received.
func (h *putHandler) closeEmpty() error {
	if err := h.req.Send(&grpc.PutResult{
		FlushLsn: 0,
		WriteLsn: 0,
	}); err != nil {
		h.logger.Error(err, "Error while closing empty WAL file")

		return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
	}

	return nil
}

// closeBuffer closes the WAL buffer, distinguishing between a partial and a
// complete WAL segment.
func (h *putHandler) closeBuffer(ctx context.Context) error {
	if h.isCompleted() {
		return h.closeComplete(ctx)
	}

	return h.closePartial()
}

// isCompleted returns true if the WAL segment has been fully received.
func (h *putHandler) isCompleted() bool {
	return h.flushedBytes == h.blockMeta.segmentSize
}

// closePartial closes a partially received WAL file.
func (h *putHandler) closePartial() error {
	if err := h.walBuffer.Close(); err != nil {
		h.logger.Warning(
			"Error while closing partial WAL file",
			"walFileName", h.blockMeta.walFileName,
			"clusterName", h.blockMeta.clusterName,
			"err", err,
		)

		return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
	}

	h.logger.Info(
		"Received partial WAL file",
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
