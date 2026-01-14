package grpcclient

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// SendBlock implements common.WALUploaderImpl.
func (g *grpcWALStream) SendBlock(ctx context.Context, block []byte) error {
	spanContext := trace.SpanFromContext(ctx).SpanContext()

	if err := g.innerStream.Send(&klioGRPC.PutRequest{
		ClusterName: g.clusterName,
		WalName:     g.walName,
		SegmentSize: g.segmentSize,
		WalBlock:    block,
		TraceId:     spanContext.TraceID().String(),
		SpanId:      spanContext.SpanID().String(),
		SendToTier2: g.sendToTier2,
	}); err != nil {
		return fmt.Errorf("error while sending WAL block (send streaming len=%v): %w", len(block), err)
	}

	g.sentBytes += uint64(len(block))

	return nil
}
