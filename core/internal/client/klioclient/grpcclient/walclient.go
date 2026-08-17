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

// StoreWAL uploads a WAL in the WAL server
// Important: this function uploads a full WAL file.
func (c *Connection) StoreWAL(ctx context.Context, name string, content []byte, sendToTier2 bool) error {
	stream, err := c.Put(ctx)
	if err != nil {
		return fmt.Errorf("while starting uploading a WAL file: %w", err)
	}

	walReader := bytes.NewBuffer(content)

	buffer := make([]byte, 4096)
	for {
		readBytes, readError := walReader.Read(buffer)
		if readError != nil && !errors.Is(readError, io.EOF) {
			return fmt.Errorf("error while reading WAL (reading from buffer): %w", readError)
		}

		if err := stream.Send(&klioGRPC.PutRequest{
			ClusterName: c.clientConfig.ClusterName,
			WalName:     name,
			SegmentSize: uint64(len(content)),
			WalBlock:    buffer[:readBytes],
			SendToTier2: sendToTier2,
		}); err != nil {
			return fmt.Errorf("error while sending WAL block (sending via GRPC): %w", err)
		}

		if errors.Is(readError, io.EOF) {
			break
		}
	}

	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("while closing the WAL upload stream: %w", err)
	}

	writtenSize, err := drainPutAcks(stream)
	if err != nil {
		return err
	}

	if writtenSize != uint64(len(content)) {
		return &IncompleteWALFileError{
			uploadedSize: writtenSize,
			expectedSize: uint64(len(content)),
		}
	}

	return nil
}

// drainPutAcks consumes the durability acknowledgements of a Put stream until
// it ends, returning the last (cumulative) durable size reported by the server.
func drainPutAcks(stream klioGRPC.WAL_PutClient) (uint64, error) {
	var writtenSize uint64
	for {
		result, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return writtenSize, nil
		}
		if err != nil {
			return 0, fmt.Errorf("while flushing WAL file: %w", err)
		}

		writtenSize = result.GetWrittenSize()
	}
}

// StoreHistoryFile uses the underlying GRPC connection to store a history file.
func (c *Connection) StoreHistoryFile(ctx context.Context, name string, content []byte, sendToTier2 bool) error {
	return c.StoreWAL(ctx, name, content, sendToTier2)
}
