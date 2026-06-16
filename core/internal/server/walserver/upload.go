package walserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
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
	span      trace.Span
	logger    log.Logger
	startTime time.Time

	blockMeta   walUploadBlockMetadata
	walBuffer   *repository.Writer
	writtenSize uint64
}

// Put uploads a new WAL to the data store.
func (w *Implementation) Put(req grpc.WAL_PutServer) error {
	if w.isReadOnly {
		return status.Error(grpccodes.FailedPrecondition, errReadOnly.Error())
	}

	ctx, span := tracer.Start(req.Context(), opentelemetry.PutWalSpan)
	defer span.End()

	h := &putHandler{
		impl:      w,
		req:       req,
		span:      span,
		logger:    log.FromContext(req.Context()),
		startTime: time.Now(),
	}

	return h.run(ctx)
}

// run consumes the stream of WAL blocks and finalizes the upload. A receive
// error (including io.EOF) stops the loop and the upload is finalized with
// whatever has been written so far.
func (h *putHandler) run(ctx context.Context) error {
	for {
		request, err := h.receiveBlock(ctx)
		if err != nil {
			break
		}

		if err := h.processBlock(ctx, request); err != nil {
			return err
		}
	}

	return h.finalize(ctx)
}

// receiveBlock reads the next WAL block from the stream, recording tracing
// information. It returns an error (including io.EOF) once the stream is
// exhausted or fails.
func (h *putHandler) receiveBlock(ctx context.Context) (*grpc.PutRequest, error) {
	_, readSpan := tracer.Start(ctx, opentelemetry.ReceiveBlockSpan)
	defer readSpan.End()

	request, err := h.req.Recv()
	if err != nil {
		if !errors.Is(err, io.EOF) {
			readSpan.SetStatus(otelcodes.Error, err.Error())
			h.logger.Warning(
				"Error while reading WAL block",
				"clusterName", request.GetClusterName(),
				"walName", request.GetWalName(),
				"err", err,
			)
		}
		readSpan.RecordError(err)

		return nil, err
	}

	readSpan.SetAttributes(attribute.Int("len", len(request.GetWalBlock())))
	addTraceLink(readSpan, request)

	return request, nil
}

// addTraceLink links the read span to the trace propagated in the request, if any.
func addTraceLink(readSpan trace.Span, request *grpc.PutRequest) {
	if request == nil || request.GetTraceId() == "" || request.GetSpanId() == "" {
		return
	}

	traceID, err := trace.TraceIDFromHex(request.GetTraceId())
	if err != nil {
		readSpan.RecordError(err)
	}
	spanID, err := trace.SpanIDFromHex(request.GetSpanId())
	if err != nil {
		readSpan.RecordError(err)
	}

	readSpan.AddLink(
		trace.Link{
			SpanContext: trace.NewSpanContext(
				trace.SpanContextConfig{
					TraceID: traceID,
					SpanID:  spanID,
				},
			),
		},
	)
}

// processBlock validates a received block, writes it to the WAL buffer and
// updates the latest-written metrics.
func (h *putHandler) processBlock(ctx context.Context, request *grpc.PutRequest) error {
	if err := h.validateRequest(request); err != nil {
		return err
	}

	h.span.SetAttributes(
		opentelemetry.AttributeKeyClusterName.Of(request.GetClusterName()),
		opentelemetry.AttributeKeyWalName.Of(request.GetWalName()),
	)

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

	h.recordLatestWrittenTimeline(ctx)

	if err := h.openWriter(request); err != nil {
		return err
	}

	if err := h.writeBlock(ctx, request); err != nil {
		return err
	}

	h.writtenSize += uint64(len(request.GetWalBlock()))
	h.recordLatestWrittenLSN(ctx)

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
			Tracer:      tracer,
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

	_, flushSpan := tracer.Start(ctx, opentelemetry.FlushBlockSpan)
	defer flushSpan.End()

	if err := h.walBuffer.Flush(); err != nil {
		h.logger.Error(
			err,
			"Error while flushing WAL data",
			"clusterName", request.GetClusterName(),
			"walName", request.GetWalName(),
		)

		flushSpan.SetStatus(otelcodes.Error, err.Error())
		flushSpan.RecordError(err)

		return status.Errorf(grpccodes.Internal, "error while flushing WAL: %v", err.Error())
	}

	return nil
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

	if err := h.req.SendAndClose(&grpc.PutResult{
		WrittenSize: h.writtenSize,
	}); err != nil {
		h.logger.Warning(
			"Error while sending WAL Put response",
			"writtenSize", h.writtenSize,
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
	if err := h.req.SendAndClose(&grpc.PutResult{
		WrittenSize: 0,
	}); err != nil {
		h.logger.Error(err, "Error while closing empty WAL file")

		return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
	}

	return nil
}

// closeBuffer closes the WAL buffer, distinguishing between a partial and a
// complete WAL segment.
func (h *putHandler) closeBuffer(ctx context.Context) error {
	if h.writtenSize != h.blockMeta.segmentSize || h.writtenSize == 0 {
		return h.closePartial()
	}

	return h.closeComplete(ctx)
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
		WALName:     h.blockMeta.walFileName,
	}); err != nil {
		return status.Errorf(
			grpccodes.Internal,
			"error while putting WAL message into the queue, please retry: %v",
			err.Error(),
		)
	}

	return nil
}
