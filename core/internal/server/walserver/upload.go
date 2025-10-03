package walserver

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

var (
	errEmptyClusterName = errors.New("empty cluster name")
	errEmptyWALName     = errors.New("empty WAL name")
	errEmptySegmentSize = errors.New("empty segment size")
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

	return nil
}

// Put uploads a new WAL to the data store.
//
//nolint:cyclop,gocognit,maintidx
func (w *Implementation) Put(req grpc.WAL_PutServer) error {
	var blockMeta walUploadBlockMetadata
	var walBuffer *Writer
	var writtenSize uint64
	startTime := time.Now()

	ctx, span := tracer.Start(req.Context(), "put_wal")
	defer span.End()

	for {
		_, readSpan := tracer.Start(ctx, opentelemetry.ReceiveBlockSpan)
		request, err := req.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readSpan.SetStatus(otelcodes.Error, err.Error())
				w.logger.Warning(
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

		if err := validatePathComponent(request.GetClusterName()); err != nil {
			w.logger.Warning("Wrong cluster name used in WAL Put", "clusterName", request.GetClusterName())
			return status.Errorf(grpccodes.InvalidArgument, "invalid cluster name: %v", err.Error())
		}

		if err := validatePathComponent(request.GetWalName()); err != nil {
			w.logger.Warning("Wrong WAL name used in WAL Put", "walName", request.GetWalName())
			return status.Errorf(grpccodes.InvalidArgument, "invalid WAL name: %v", err.Error())
		}

		if err := validateWalFileName(request.GetWalName()); err != nil {
			return status.Errorf(grpccodes.InvalidArgument, "invalid WAL name: %q", request.GetWalName())
		}

		span.SetAttributes(attribute.String("clusterName", request.GetClusterName()))
		span.SetAttributes(attribute.String("walName", request.GetWalName()))

		w.logger.Debug(
			"Received WAL block",
			"clusterName", request.GetClusterName(),
			"walName", request.GetWalName(),
			"blockLen", len(request.GetWalBlock()),
		)

		if err := blockMeta.handleRequest(request); err != nil {
			w.logger.Error(
				err,
				"Incoherent WAL Block received",
				"clusterName", request.GetClusterName(),
				"walName", request.GetWalName(),
			)

			return status.Errorf(grpccodes.InvalidArgument, "%s", err.Error())
		}

		if walBuffer == nil {
			walBuffer, err = NewWriter(w.conn, blockMeta.clusterName, blockMeta.walFileName, blockMeta.segmentSize,
				w.metrics)
			if err != nil {
				w.logger.Error(
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
			w.logger.Error(
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
			w.logger.Error(
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
	}

	if walBuffer == nil {
		if err := req.SendAndClose(&grpc.PutResult{
			WrittenSize: 0,
		}); err != nil {
			w.logger.Error(
				err,
				"Error while closing empty WAL file",
			)

			return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
		}

		return nil
	}

	if writtenSize != blockMeta.segmentSize || writtenSize == 0 {
		if err := walBuffer.Close(); err != nil {
			w.logger.Warning(
				"Error while closing partial WAL file",
				"writtenSize", writtenSize,
				"walFileName", blockMeta.walFileName,
				"clusterName", blockMeta.clusterName,
				"err", err,
			)

			return status.Errorf(grpccodes.Internal, "error while closing (partial) WAL: %v", err.Error())
		}

		w.logger.Info(
			"Received partial WAL file",
			"writtenSize", writtenSize,
			"segmentSize", blockMeta.segmentSize,
			"walFileName", blockMeta.walFileName,
			"clusterName", blockMeta.clusterName,
			"elapsedTime", time.Since(startTime),
		)
	} else {
		if err := walBuffer.CloseMarkDone(); err != nil {
			w.logger.Warning(
				"Error while closing completed WAL file",
				"writtenSize", writtenSize,
				"walFileName", blockMeta.walFileName,
				"clusterName", blockMeta.clusterName,
				"err", err,
			)

			return status.Errorf(grpccodes.Internal, "error while closing (done) WAL: %v", err.Error())
		}

		w.metrics.walWritten.Add(
			req.Context(),
			1,
			metric.WithAttributeSet(
				attribute.NewSet(
					attribute.String("cluster_name", blockMeta.clusterName),
				),
			),
		)

		w.logger.Info(
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
		w.logger.Warning(
			"Error while sending WAL Put response",
			"writtenSize", writtenSize,
			"walFileName", blockMeta.walFileName,
			"clusterName", blockMeta.clusterName,
			"err", err,
		)

		return status.Errorf(grpccodes.Internal, "error while sending response: %v", err.Error())
	}

	return nil
}
