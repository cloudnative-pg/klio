package opentelemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	// DownloadHistoryFileSpan is the span name for downloading history files.
	DownloadHistoryFileSpan = "download_history_file"
	// ManageWalStreamSpan is the span name for managing WAL streams.
	ManageWalStreamSpan = "manage_wal_stream"
	// FlushBlockSpan is the span name for flushing blocks.
	FlushBlockSpan = "flush_block"
	// ReceiveBlockSpan is the span name for receiving blocks.
	ReceiveBlockSpan = "receive_block"
	// WriteBlockSpan is the span name for writing blocks.
	WriteBlockSpan = "write_block"
	// ReadBlockSpan is the span name for reading blocks.
	ReadBlockSpan = "read_block"
	// ReadBlockDataSpan is the span name for reading block data.
	ReadBlockDataSpan = "read_block_data"
	// WriteBlockDataSpan is the span name for writing block data.
	WriteBlockDataSpan = "write_block_data"
	// WrapBlockSpan is the span name for wrapping block data.
	WrapBlockSpan = "wrap_block_data"
	// UnwrapBlockSpan is the span name for unwrapping block data.
	UnwrapBlockSpan = "unwrap_block_data"
	// SendBlockSpan is the span name for sending blocks.
	SendBlockSpan = "send_block"
	// GetWalSpan is the span name for getting WAL files.
	GetWalSpan = "get_wal"
)

// newTracerProvider creates a new OpenTelemetry TracerProvider.
func newTracerProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	res, err := createResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	exp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create span exporter: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	), nil
}
