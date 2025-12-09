package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// StoreWALStreaming implements the WAL streaming service.
func (c *Connection) StoreWALStreaming(
	ctx context.Context,
	name string,
	segmentSize uint64,
) (*common.WALUploader, error) {
	stream, err := c.Put(ctx)
	if err != nil {
		return nil, fmt.Errorf("while starting uploading a WAL file: %w", err)
	}

	return common.NewWALUploader(&grpcWALStream{
		innerStream: stream,
		segmentSize: segmentSize,
		clusterName: c.cfg.ClusterName,
		walName:     name,
	}), nil
}

// GetWALStreaming get a WAL from a remote connection.
func (c *Connection) GetWALStreaming(ctx context.Context, walName string, out io.Writer) error { //nolint:cyclop
	client, err := c.Get(ctx, &klioGRPC.GetRequest{
		ClusterName: c.cfg.ClusterName,
		WalName:     walName,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return common.ErrMissingWALFile
		}

		return fmt.Errorf("while starting downloading a WAL file: %w", err)
	}

	var expectedSize, writtenBytes int
	for {
		result, err := client.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			if status.Code(err) == codes.NotFound {
				return common.ErrMissingWALFile
			}

			if writtenBytes > 0 {
				return common.IncompleteTransmissionError{
					Inner:        err,
					WrittenBytes: writtenBytes,
				}
			}

			return fmt.Errorf("while receiving a WAL file block: %w", err)
		}

		if expectedSize == 0 {
			expectedSize = int(result.GetSegmentSize())
		}

		b, err := out.Write(result.GetWalBlock())
		if err != nil {
			return fmt.Errorf("while writing WAL file: %w", err)
		}

		writtenBytes += b
	}

	// If this is a partial WAL, we pad it until the target WAL size is reached
	if strings.HasSuffix(walName, ".partial") && expectedSize > writtenBytes {
		if err := c.padWithZeros(out, expectedSize-writtenBytes); err != nil {
			return err
		}
	}

	return nil
}

func (c *Connection) padWithZeros(out io.Writer, zeroBytesToWrite int) error {
	blockSize := 1024 * 1024
	zeroBlock := make([]byte, blockSize)

	totalWritten := 0
	for totalWritten < zeroBytesToWrite {
		toWrite := min(blockSize, zeroBytesToWrite-totalWritten)

		n, err := out.Write(zeroBlock[:toWrite])
		if err != nil {
			return fmt.Errorf("while padding WAL file: %w", err)
		}

		totalWritten += n
	}

	return nil
}
