package walserver

import (
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

// Put uploads a new WAL to the data store.
//
//nolint:cyclop,gocognit,maintidx,gocyclo
func (w *Implementation) Put(req grpc.WAL_PutServer) error {
	var blockMeta walUploadBlockMetadata
	var walBuffer *repository.Writer
	var writtenSize uint64

	if w.isReadOnly {
		return status.Error(grpccodes.FailedPrecondition, errReadOnly.Error())
	}

	startTime := time.Now()

	logger := log.FromContext(req.Context())

	ctx, span := tracer.Start(req.Context(), opentelemetry.PutWalSpan)
	defer span.End()

	for {
		_, readSpan := tracer.Start(ctx, opentelemetry.ReceiveBlockSpan)
		request, err := req.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readSpan.SetStatus(otelcodes.Error, err.Error())
				logger.Warning(
					"Error while reading WAL block",
					"clusterName", request.GetClusterName(),
					"walName", request.GetWalName(),
					"err", err,
				)
			}
			readSpan.RecordError(err)
			readSpan.End()

			break
		}

		readSpan.SetAttributes(
			attribute.Int("len", len(request.GetWalBlock())),
		)

		if request != nil && request.GetTraceId() != "" && request.GetSpanId() != "" {
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
		readSpan.End()

		if errors.Is(err, io.EOF) {
			break
		}

		if err := repository.ValidatePathComponent(request.GetClusterName()); err != nil {
			logger.Warning("Wrong cluster name used in WAL Put", "clusterName", request.GetClusterName())
			return status.Errorf(grpccodes.InvalidArgument, "invalid cluster name: %v", err.Error())
		}

		if err := repository.ValidatePathComponent(request.GetWalName()); err != nil {
			logger.Warning("Wrong WAL name used in WAL Put", "walName", request.GetWalName())
			return status.Errorf(grpccodes.InvalidArgument, "invalid WAL name: %v", err.Error())
		}

		if err := repository.ValidateWalFileName(request.GetWalName()); err != nil {
			return status.Errorf(grpccodes.InvalidArgument, "invalid WAL name: %q", request.GetWalName())
		}

		span.SetAttributes(attribute.String("clusterName", request.GetClusterName()))
		span.SetAttributes(attribute.String("walName", request.GetWalName()))

		logger.Debug(
			"Received WAL block",
			"clusterName", request.GetClusterName(),
			"walName", request.GetWalName(),
			"blockLen", len(request.GetWalBlock()),
		)

		if err := blockMeta.handleRequest(request); err != nil {
			logger.Error(
				err,
				"Incoherent WAL Block received",
				"clusterName", request.GetClusterName(),
				"walName", request.GetWalName(),
			)

			return status.Errorf(grpccodes.InvalidArgument, "%s", err.Error())
		}

		if segment, err := postgres.SegmentFromName(blockMeta.walFileName); err != nil {
			logger.Error(err, "Could not parse timeline for latest written timeline metric",
				"walName", blockMeta.walFileName)
		} else {
			w.metrics.LatestWrittenTimeline.Record(
				ctx,
				int64(segment.Tli),
				metric.WithAttributeSet(
					attribute.NewSet(
						attribute.String("cluster_name", blockMeta.clusterName),
					),
				),
			)
		}

		if walBuffer == nil {
			walBuffer, err = w.conn.NewWriter(blockMeta.clusterName, blockMeta.walFileName, blockMeta.segmentSize,
				w.metrics, tracer)
			if err != nil {
				logger.Error(
					err,
					"Cannot open new WAL file",
					"clusterName", request.GetClusterName(),
					"walName", request.GetWalName(),
				)

				return status.Errorf(grpccodes.Internal, "error while opening new WAL: %v", err.Error())
			}
		}

		err = walBuffer.WriteBlock(ctx, request.GetWalBlock())
		if err != nil {
			logger.Error(
				err,
				"Error while writing WAL data",
				"clusterName", request.GetClusterName(),
				"walName", request.GetWalName(),
			)

			return status.Errorf(grpccodes.Internal, "error while writing WAL: %v", err.Error())
		}

		_, flushSpan := tracer.Start(ctx, opentelemetry.FlushBlockSpan)
		err = walBuffer.Flush()
		if err != nil {
			logger.Error(
				err,
				"Error while flushing WAL data",
				"clusterName", request.GetClusterName(),
				"walName", request.GetWalName(),
			)

			flushSpan.SetStatus(otelcodes.Error, err.Error())
			flushSpan.RecordError(err)
			flushSpan.End()

			return status.Errorf(grpccodes.Internal, "error while flushing WAL: %v", err.Error())
		}
		flushSpan.End()

		writtenSize += uint64(len(request.GetWalBlock()))

		if startLSN, err := types.LSNStartFromWALName(blockMeta.walFileName, blockMeta.segmentSize); err != nil {
			logger.Error(err, "Could not compute start LSN for latest written LSN metric",
				"walName", blockMeta.walFileName)
		} else if startPos, parseErr := startLSN.Parse(); parseErr != nil {
			logger.Error(parseErr, "Could not parse start LSN for latest written LSN metric",
				"walName", blockMeta.walFileName)
		} else {
			w.metrics.LatestWrittenLSN.Record(
				ctx,
				int64(startPos+writtenSize), //nolint:gosec
				metric.WithAttributeSet(
					attribute.NewSet(
						attribute.String("cluster_name", blockMeta.clusterName),
					),
				),
			)
		}
	}

	if walBuffer == nil {
		if err := req.SendAndClose(&grpc.PutResult{
			WrittenSize: 0,
		}); err != nil {
			logger.Error(
				err,
				"Error while closing empty WAL file",
			)

			return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
		}

		return nil
	}

	if writtenSize != blockMeta.segmentSize || writtenSize == 0 {
		if err := walBuffer.Close(); err != nil {
			logger.Warning(
				"Error while closing partial WAL file",
				"writtenSize", writtenSize,
				"walFileName", blockMeta.walFileName,
				"clusterName", blockMeta.clusterName,
				"err", err,
			)

			return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
		}

		logger.Info(
			"Received partial WAL file",
			"writtenSize", writtenSize,
			"segmentSize", blockMeta.segmentSize,
			"walFileName", blockMeta.walFileName,
			"clusterName", blockMeta.clusterName,
			"elapsedTime", time.Since(startTime),
		)
	} else {
		if err := walBuffer.CloseMarkDone(); err != nil {
			logger.Warning(
				"Error while closing completed WAL file",
				"writtenSize", writtenSize,
				"walFileName", blockMeta.walFileName,
				"clusterName", blockMeta.clusterName,
				"err", err,
			)

			return status.Errorf(grpccodes.Internal, "error while closing (done) WAL: %v", err.Error())
		}

		clusterAttr := metric.WithAttributeSet(
			attribute.NewSet(
				attribute.String("cluster_name", blockMeta.clusterName),
			),
		)

		w.metrics.WalWritten.Add(req.Context(), 1, clusterAttr)
		w.metrics.LatestWrittenTime.Record(req.Context(), float64(time.Now().Unix()), clusterAttr)

		logger.Info(
			"Received completed WAL file",
			"writtenSize", writtenSize,
			"segmentSize", blockMeta.segmentSize,
			"walFileName", blockMeta.walFileName,
			"clusterName", blockMeta.clusterName,
			"elapsedTime", time.Since(startTime),
		)
	}

	if err := req.SendAndClose(&grpc.PutResult{
		WrittenSize: writtenSize,
	}); err != nil {
		logger.Warning(
			"Error while sending WAL Put response",
			"writtenSize", writtenSize,
			"walFileName", blockMeta.walFileName,
			"clusterName", blockMeta.clusterName,
			"err", err,
		)

		return status.Errorf(grpccodes.Internal, "error while sending response: %v", err.Error())
	}

	if blockMeta.sendToTier2 {
		if w.queue == nil {
			return status.Errorf(
				grpccodes.Internal,
				"queue service is uninitialized",
			)
		}

		if err := w.queue.NotifyWALReceived(ctx, &queue.WALTask{
			ClusterName: blockMeta.clusterName,
			WALName:     blockMeta.walFileName,
		}); err != nil {
			return status.Errorf(
				grpccodes.Internal,
				"error while putting WAL message into the queue, please retry: %v",
				err.Error(),
			)
		}
	}

	return nil
}
