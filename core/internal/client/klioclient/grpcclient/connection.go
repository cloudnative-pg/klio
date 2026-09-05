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
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/wal"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Connection represents a connection to a Klio server.
type Connection struct {
	klioGRPC.WALClient

	clientConfig   *config.ClientConfig
	grpcConnection *grpc.ClientConn
}

// StoreWALStreaming implements the WAL streaming service.
//
//nolint:ireturn
func (c *Connection) StoreWALStreaming(
	ctx context.Context,
	name string,
	segmentSize uint64,
	sendToTier2 bool,
	walStartLSN uint64,
	feedbackChannel chan<- wal.Feedback,
) (klioclient.WALUploader, error) {
	stream, err := c.Put(ctx)
	if err != nil {
		return nil, fmt.Errorf("while starting uploading a WAL file: %w", err)
	}

	g := &grpcWALStream{
		innerStream:     stream,
		segmentSize:     segmentSize,
		clusterName:     c.clientConfig.ClusterName,
		walName:         name,
		sendToTier2:     sendToTier2,
		walStartLSN:     walStartLSN,
		feedbackChannel: feedbackChannel,
	}
	g.startFeedbackReader()

	return g, nil
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
