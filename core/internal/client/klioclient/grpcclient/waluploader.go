package grpcclient

import (
	"context"
	"fmt"
	"time"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// SendBlock implements common.WALUploaderImpl.
func (g *grpcWALStream) SendBlock(ctx context.Context, block []byte) error {
	sendStart := time.Now()
	err := g.innerStream.Send(&klioGRPC.PutRequest{
		ClusterName: g.clusterName,
		WalName:     g.walName,
		SegmentSize: g.segmentSize,
		WalBlock:    block,
		SendToTier2: g.sendToTier2,
	})

	opentelemetry.RecordDuration(ctx, opentelemetry.ClientWal.BlockDuration, time.Since(sendStart), err,
		opentelemetry.AttributeKeyClusterName.Of(g.clusterName),
		opentelemetry.PathPut.Attribute(), opentelemetry.StageSend.Attribute())

	if err != nil {
		return fmt.Errorf("error while sending WAL block (send streaming len=%v): %w", len(block), err)
	}

	g.sentBytes += uint64(len(block))

	return nil
}
