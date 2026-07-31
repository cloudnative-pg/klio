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
