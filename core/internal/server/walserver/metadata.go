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

package walserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/protobuf/proto"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// ErrIncoherentMetadata happens when the cluster metadata is incoherent
// or not unmarshallable.
var ErrIncoherentMetadata = errors.New("incoherent cluster metadata")

// getClusterMetadata gets the cluster metadata for the cluster with the passed name.
func (w *Implementation) getClusterMetadata(ctx context.Context, clusterName string) (*grpc.ClusterMetadata, error) {
	logger := log.FromContext(ctx)

	// Metadata is not WAL data, so pass nil metrics to skip per-block recording.
	walReader, err := repository.NewReader(w.conn, clusterName, metadataFileName, nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := walReader.Close(); err != nil {
			logger.Error(err, "Error while closing metadata file for read", "clusterName", clusterName)
		}
	}()

	data, err := walReader.ReadBlock(ctx)
	if err != nil {
		return nil, err
	}

	var metadata grpc.ClusterMetadata
	if err := proto.Unmarshal(data, &metadata); err != nil {
		return nil, ErrIncoherentMetadata
	}

	return &metadata, nil
}

// writeClusterMetadata gets the cluster metadata for the cluster with the passed name.
func (w *Implementation) writeClusterMetadata(
	ctx context.Context,
	clusterName string,
	metadata *grpc.ClusterMetadata,
) error {
	logger := log.FromContext(ctx)
	data, err := proto.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("internal error while marshalling protobuf data: %w", err)
	}

	walWriter, err := w.conn.NewWriter(
		repository.WriterOptions{
			ClusterName: clusterName,
			WALName:     metadataFileName,
			SegmentSize: uint64(len(data)),
			Metrics:     w.metrics,
		},
	)
	if err != nil {
		return err
	}

	defer func() {
		if err := walWriter.CloseMarkDone(); err != nil {
			logger.Error(err, "Error while closing metadata file for read", "clusterName", clusterName)
		}
	}()

	if err := walWriter.WriteBlock(ctx, data); err != nil {
		return err
	}

	return nil
}
