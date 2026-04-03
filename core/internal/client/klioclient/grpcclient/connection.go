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

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

type grpcWALStream struct {
	innerStream klioGRPC.WAL_PutClient
	segmentSize uint64
	clusterName string
	sentBytes   uint64
	walName     string
	sendToTier2 bool
}

// Close implements common.WALStream.
func (g *grpcWALStream) Close(_ context.Context) error {
	result, err := g.innerStream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("while flushing WAL file: %w", err)
	}

	if result.GetWrittenSize() != g.sentBytes {
		return &IncompleteWALFileError{
			uploadedSize: result.GetWrittenSize(),
			expectedSize: g.sentBytes,
		}
	}

	return nil
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
		grpc.WithInitialWindowSize(256*1024),
		grpc.WithInitialConnWindowSize(256*1024),
		grpc.WithReadBufferSize(256*1024),
		grpc.WithWriteBufferSize(256*1024),
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
