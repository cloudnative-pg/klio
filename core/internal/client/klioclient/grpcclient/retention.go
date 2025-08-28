package grpcclient

import (
	"context"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// SetFirstRequiredWAL drops every WAL file older than the passed WAL file name.
func (c *Connection) SetFirstRequiredWAL(ctx context.Context, walName string) error {
	_, err := c.WALClient.SetFirstRequiredWAL(ctx, &grpc.SetFirstRequiredWALRequest{
		ClusterName:      c.cfg.ClusterName,
		FirstRequiredWal: walName,
	})

	return err
}
