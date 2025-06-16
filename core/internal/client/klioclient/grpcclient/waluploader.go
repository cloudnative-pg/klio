package grpcclient

import (
	"context"
	"fmt"

	klioGRPC "github.com/EnterpriseDB/klio/internal/grpc"
)

// SendBlock implements common.WALUploaderImpl.
func (g *grpcWALStream) SendBlock(_ context.Context, block []byte) error {
	if err := g.innerStream.Send(&klioGRPC.PutRequest{
		ClusterName: g.clusterName,
		WalName:     g.walName,
		SegmentSize: g.segmentSize,
		WalBlock:    block,
	}); err != nil {
		return fmt.Errorf("error while sending WAL block (send streaming len=%v): %w", len(block), err)
	}

	g.sentBytes += uint64(len(block))

	return nil
}
