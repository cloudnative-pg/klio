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
	"io"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

const retentionPolicyFileName = "retention_policy"

// SetRetentionPolicy implements the SetRetentionPolicy GRPC call.
func (w *Implementation) SetRetentionPolicy(
	ctx context.Context, request *grpc.SetRetentionPolicyRequest,
) (*grpc.SetRetentionPolicyResult, error) {
	if err := repository.ValidatePathComponent(request.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := authorizeClusterName(ctx, request.GetClusterName()); err != nil {
		return nil, err
	}

	logger := log.FromContext(ctx)
	retentionPolicy, err := proto.Marshal(request.GetRetentionPolicy())
	if err != nil {
		return nil, fmt.Errorf("internal error while marshalling protobuf data: %w", err)
	}

	// Retention policy is not WAL data, so pass nil metrics: it must not
	// count toward klio.server.wal.written_size.
	walWriter, err := w.conn.NewWriter(
		repository.WriterOptions{
			ClusterName: request.GetClusterName(),
			WALName:     retentionPolicyFileName,
			SegmentSize: uint64(len(retentionPolicy)),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("internal error while creating WAL writer: %w", err)
	}

	defer func() {
		if err := walWriter.CloseMarkDone(); err != nil {
			logger.Error(err, "Error while closing policy file", "clusterName", request.GetClusterName())
		}
	}()

	if err := walWriter.WriteBlock(ctx, retentionPolicy); err != nil {
		return nil, fmt.Errorf("internal error while writing WAL block: %w", err)
	}

	return &grpc.SetRetentionPolicyResult{}, nil
}

// GetRetentionPolicy implements the GetRetentionPolicy GRPC call.
func (w *Implementation) GetRetentionPolicy(
	ctx context.Context, request *grpc.GetRetentionPolicyRequest,
) (*grpc.GetRetentionPolicyResult, error) {
	if err := repository.ValidatePathComponent(request.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := authorizeClusterName(ctx, request.GetClusterName()); err != nil {
		return nil, err
	}

	var policy *grpc.RetentionPolicy
	var err error

	if policy, err = w.getClusterRetentionPolicy(ctx, request.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.Internal, "error while reading cluster retention policy: %v", err.Error())
	}

	return &grpc.GetRetentionPolicyResult{RetentionPolicy: policy}, nil
}

// getClusterRetentionPolicy gets the cluster retention policy for the
// cluster with the passed name.
func (w *Implementation) getClusterRetentionPolicy(
	ctx context.Context, clusterName string,
) (*grpc.RetentionPolicy, error) {
	return GetClusterRetentionPolicy(ctx, w.conn, clusterName)
}

// GetClusterRetentionPolicy reads the retention policy stored for
// clusterName in conn, the WAL repository backing a WAL server. Exported so
// the retention sweep (internal/retention) can read it in-process, sharing
// the same tier1 WAL repository as this server, without a gRPC round trip.
func GetClusterRetentionPolicy(
	ctx context.Context, conn *repository.Connection, clusterName string,
) (*grpc.RetentionPolicy, error) {
	logger := log.FromContext(ctx)

	// Retention policy is not WAL data, so pass nil metrics to skip per-block recording.
	walReader, err := repository.NewReader(conn, clusterName, retentionPolicyFileName, nil)
	if err != nil {
		// The file was never created: no policy has ever been set for this
		// cluster (e.g. no backup has completed yet, so klio retention set
		// was never called).
		if errors.Is(err, os.ErrNotExist) {
			return &grpc.RetentionPolicy{}, nil
		}

		return nil, err
	}

	defer func() {
		if err := walReader.Close(); err != nil {
			logger.Error(err, "Error while closing policy file for read", "clusterName", clusterName)
		}
	}()

	data, err := walReader.ReadBlock(ctx)
	if err != nil {
		// The file exists but has no block after its header: this is how a
		// policy with neither tier set is actually written, since
		// proto.Marshal of an all-nil RetentionPolicy is zero bytes, and
		// WriteBlock skips writing anything at all for an empty block. Also
		// means "no policy configured".
		if errors.Is(err, io.EOF) {
			return &grpc.RetentionPolicy{}, nil
		}

		return nil, err
	}

	var policy grpc.RetentionPolicy
	if err := proto.Unmarshal(data, &policy); err != nil {
		return nil, ErrIncoherentMetadata
	}

	return &policy, nil
}
