package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"

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
	logger *slog.Logger
	cfg    *config.WalRepositoryClientConfig

	klioGRPC.WALClient
	grpcConnection *grpc.ClientConn
}

// Connect opens a connection to a Klio server.
func Connect(logger *slog.Logger, cfg *config.WalRepositoryClientConfig) (*Connection, error) {
	certPEMBlock, err := os.ReadFile(cfg.ServerCertPath)
	if err != nil {
		return nil, fmt.Errorf("while reading the server certificate: %w", err)
	}

	serverCertificatePool := x509.NewCertPool()
	if !serverCertificatePool.AppendCertsFromPEM(certPEMBlock) {
		return nil, ErrInconsistentCertificate
	}

	tlsConfig := &tls.Config{
		RootCAs:    serverCertificatePool,
		MinVersion: tls.VersionTLS13,
	}

	conn, err := grpc.NewClient(
		cfg.Address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithPerRPCCredentials(&basicAuthCredentials{
			username: fmt.Sprintf("%s@%s", cfg.Username, cfg.ClusterName),
			password: cfg.Password,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("while establishing connection to the server: %w", err)
	}

	walClient := klioGRPC.NewWALClient(conn)

	return &Connection{
		logger:         logger,
		cfg:            cfg,
		WALClient:      walClient,
		grpcConnection: conn,
	}, nil
}
