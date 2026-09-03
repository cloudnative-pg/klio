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
	"time"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/wal"
)

// grpcWALStream uploads a WAL file to a Klio server over a single
// bidirectional gRPC stream.
//
// A background goroutine reads the acknowledgments and streams them
// to feedbackChannel.
type grpcWALStream struct {
	innerStream klioGRPC.WAL_PutClient
	segmentSize uint64
	clusterName string
	sentBytes   uint64
	walName     string
	walStartLSN uint64
	sendToTier2 bool

	feedbackChannel chan<- wal.Feedback

	// ackErr and ackDone follow the same pattern as messageReceiver.err in
	// nonblocking_receive.go: ackErr is written at most once, by the
	// background reader, always before it closes ackDone. The Go memory
	// model guarantees that observing ackDone closed happens after that
	// write, so reading ackErr once ackDone is (or is seen to be) closed is
	// safe without any extra synchronization.
	ackErr  error
	ackDone chan struct{}
}

// SendBlock implements klioclient.WALUploader. It pipelines: the block is
// handed to gRPC and SendBlock returns without waiting for the server's
// per-block acknowledgment. The background goroutine started by
// startFeedbackReader pushes each acknowledgment to feedbackChannel as it
// arrives, so the caller learns how much of what has been sent is actually
// confirmed durable from there.
func (g *grpcWALStream) SendBlock(ctx context.Context, block []byte) error {
	if err := g.ackedErr(); err != nil {
		return err
	}

	sendStart := time.Now()

	err := g.innerStream.Send(&klioGRPC.PutRequest{
		ClusterName: g.clusterName,
		WalName:     g.walName,
		SegmentSize: g.segmentSize,
		WalBlock:    block,
		SendToTier2: g.sendToTier2,
		WalStartLsn: g.walStartLSN,
	})

	sendDuration := time.Since(sendStart)

	opentelemetry.RecordDuration(ctx, opentelemetry.ClientWal.BlockDuration, sendDuration, err,
		opentelemetry.AttributeKeyClusterName.Of(g.clusterName),
		opentelemetry.PathPut.Attribute(), opentelemetry.StageSend.Attribute())

	if err != nil {
		return fmt.Errorf("error while sending WAL block (send streaming len=%v): %w", len(block), err)
	}

	g.sentBytes += uint64(len(block))

	return nil
}

// Close stops sending on the stream and waits for the background ack
// reader to observe the end of the stream, surfacing any error it recorded.
func (g *grpcWALStream) Close(_ context.Context) error {
	if err := g.innerStream.CloseSend(); err != nil {
		return err //nolint:wrapcheck
	}

	<-g.ackDone

	return g.ackErr
}

// startFeedbackReader starts the background goroutine that drains the server's
// per-block acknowledgments.
func (g *grpcWALStream) startFeedbackReader() {
	g.ackDone = make(chan struct{})

	go func() {
		defer close(g.ackDone)

		for {
			result, err := g.innerStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					g.ackErr = fmt.Errorf("error while receiving WAL block ack: %w", err)
				}

				return
			}

			if g.feedbackChannel != nil {
				g.feedbackChannel <- wal.Feedback{
					FlushLSN:  result.GetFlushLsn(),
					WriteLSN:  result.GetWriteLsn(),
					ReplayLSN: result.GetFlushLsn(),
				}
			}
		}
	}()
}

// ackedErr returns the error recorded by the background ack reader once it
// has observed the end of the stream, so SendBlock can fail fast instead of
// sending into a stream already known to be dead.
func (g *grpcWALStream) ackedErr() error {
	select {
	case <-g.ackDone:
		return g.ackErr
	default:
		return nil
	}
}
