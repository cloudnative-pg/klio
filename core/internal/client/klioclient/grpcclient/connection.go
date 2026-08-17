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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/wal"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

type grpcWALStream struct {
	innerStream klioGRPC.WAL_PutClient
	segmentSize uint64
	clusterName string
	sentBytes   uint64
	walName     string
	sendToTier2 bool

	// syncedBytes is the highest cumulative durable size acknowledged by the
	// server for this WAL file. It is written by the ack reader goroutine and
	// read by the WAL processing goroutine, so it must be accessed atomically.
	syncedBytes atomic.Uint64

	// ackDone is closed when the ack reader goroutine has consumed every
	// acknowledgement up to the end of the stream. ackErr, set before ackDone
	// is closed, carries any non-EOF error observed while reading.
	ackDone chan struct{}
	ackErr  error
}

// newGRPCWALStream wraps a freshly opened Put stream and starts the background
// goroutine that consumes the server durability acknowledgements.
func newGRPCWALStream(
	stream klioGRPC.WAL_PutClient,
	name string,
	segmentSize uint64,
	clusterName string,
	sendToTier2 bool,
) *grpcWALStream {
	g := &grpcWALStream{
		innerStream: stream,
		segmentSize: segmentSize,
		clusterName: clusterName,
		walName:     name,
		sendToTier2: sendToTier2,
		ackDone:     make(chan struct{}),
	}

	go g.readAcks()

	return g
}

// SyncedOffset implements common.WALStream. It returns the highest durable size
// acknowledged so far for this WAL file.
func (g *grpcWALStream) SyncedOffset() (uint64, error) {
	// If the reader has already terminated with a non-EOF error, the
	// acknowledgements can no longer be trusted.
	select {
	case <-g.ackDone:
		if g.ackErr != nil {
			return 0, fmt.Errorf("while receiving WAL acknowledgements: %w", g.ackErr)
		}
	default:
	}

	return g.syncedBytes.Load(), nil
}

// Close implements common.WALStream. It stops sending, waits for every
// acknowledgement to be drained and verifies that the whole file is durable.
func (g *grpcWALStream) Close(_ context.Context) error {
	if err := g.innerStream.CloseSend(); err != nil {
		return fmt.Errorf("while closing the WAL upload stream: %w", err)
	}

	// Wait for the reader to drain all acknowledgements up to the end of the
	// stream, so syncedBytes reflects the final durable size.
	<-g.ackDone
	if g.ackErr != nil {
		return fmt.Errorf("while receiving WAL acknowledgements: %w", g.ackErr)
	}

	if synced := g.syncedBytes.Load(); synced != g.sentBytes {
		return &IncompleteWALFileError{
			uploadedSize: synced,
			expectedSize: g.sentBytes,
		}
	}

	return nil
}

// readAcks consumes the server acknowledgements until the stream ends, tracking
// the latest durable size. gRPC allows a single concurrent reader alongside the
// single writer used by SendBlock.
func (g *grpcWALStream) readAcks() {
	defer close(g.ackDone)

	for {
		result, err := g.innerStream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				g.ackErr = err
			}

			return
		}

		g.syncedBytes.Store(result.GetWrittenSize())
	}
}

// Connection represents a connection to a Klio server.
type Connection struct {
	klioGRPC.WALClient

	clientConfig   *config.ClientConfig
	grpcConnection *grpc.ClientConn
}

// Connect opens a connection to a Klio server.
func Connect(clientConfig *config.ClientConfig, address string) (*Connection, error) {
	certPEMBlock, err := os.ReadFile(clientConfig.Wal.ServerCertPath)
	if err != nil {
		return nil, fmt.Errorf("while reading the server certificate: %w", err)
	}

	serverCertificatePool := x509.NewCertPool()
	if !serverCertificatePool.AppendCertsFromPEM(certPEMBlock) {
		return nil, ErrInconsistentCertificate
	}

	clientCertificate, err := tls.LoadX509KeyPair(clientConfig.Wal.ClientCertPath, clientConfig.Wal.ClientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("while parsing the client certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		RootCAs:    serverCertificatePool,
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{
			clientCertificate,
		},
	}

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithInitialWindowSize(wal.GRPCInitialWindowSizeBytes),
		grpc.WithInitialConnWindowSize(wal.GRPCInitialConnWindowSizeBytes),
		grpc.WithReadBufferSize(wal.GRPCSocketBufferSizeBytes),
		grpc.WithWriteBufferSize(wal.GRPCSocketBufferSizeBytes),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("while establishing connection to the server: %w", err)
	}

	walClient := klioGRPC.NewWALClient(conn)

	return &Connection{
		clientConfig:   clientConfig,
		WALClient:      walClient,
		grpcConnection: conn,
	}, nil
}
