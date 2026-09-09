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

package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// GetWALStreaming get a WAL from a remote connection.
func (c *Connection) GetWALStreaming(ctx context.Context, walName string, out io.Writer) error { //nolint:cyclop
	client, err := c.Get(ctx, &klioGRPC.GetRequest{
		ClusterName: c.clientConfig.ClusterName,
		WalName:     walName,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return klioclient.ErrMissingWALFile
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
				return klioclient.ErrMissingWALFile
			}

			if writtenBytes > 0 {
				return klioclient.IncompleteTransmissionError{
					Inner:        err,
					WrittenBytes: writtenBytes,
				}
			}

			return fmt.Errorf("while receiving a WAL file block: %w", err)
		}

		if expectedSize == 0 {
			expectedSize, err = safecast.Convert[int](result.GetSegmentSize())
			if err != nil {
				return fmt.Errorf("while converting segment size: %w", err)
			}
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
