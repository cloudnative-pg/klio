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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// UploadFile uploads a file to the Klio server.
// No feedback is expected from the server.
func (c *Connection) UploadFile(ctx context.Context, name string, content []byte, sendToTier2 bool) error {
	stream, err := c.Put(ctx)
	if err != nil {
		return fmt.Errorf("while starting uploading a file: %w", err)
	}

	walReader := bytes.NewBuffer(content)
	buffer := make([]byte, 4096)

	for {
		readBytes, readError := walReader.Read(buffer)
		if readError != nil && !errors.Is(readError, io.EOF) {
			return fmt.Errorf("error while reading file (reading from buffer): %w", readError)
		}

		if err := stream.Send(&klioGRPC.PutRequest{
			ClusterName: c.clientConfig.ClusterName,
			WalName:     name,
			SegmentSize: uint64(len(content)),
			WalBlock:    buffer[:readBytes],
			SendToTier2: sendToTier2,

			// We're not interested to the feedback from the Klio server,
			// so we just start this WAL file from LSN zero.
			WalStartLsn: 0,
		}); err != nil {
			return fmt.Errorf("error while sending file block (sending via GRPC): %w", err)
		}

		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return ErrNoResultReceived
		}
		if err != nil {
			return fmt.Errorf("while flushing WAL file: %w", err)
		}

		if errors.Is(readError, io.EOF) {
			break
		}
	}

	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("while flushing WAL file: %w", err)
	}

	return nil
}
