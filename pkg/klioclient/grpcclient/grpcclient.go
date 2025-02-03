package grpcclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	klioGRPC "github.com/EnterpriseDB/klio/internal/klioserver/grpc"
	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient/types"
)

// Connection represent a connection to a Klio server.
type Connection struct {
	logger *slog.Logger
	cfg    *config.KlioRepositoryClientConfig

	walClient      klioGRPC.WALClient
	grpcConnection *grpc.ClientConn
}

// IncompleteWALFileError is raised when a WAL file has been uploaded incompletely.
type IncompleteWALFileError struct {
	uploadedSize uint64
	expectedSize uint64
}

// Error implements the error interface.
func (e *IncompleteWALFileError) Error() string {
	return fmt.Sprintf("uploaded %v expected %v", e.uploadedSize, e.expectedSize)
}

// ErrInconsistentCertificate is raised when the server certificate cannot be parsed.
var ErrInconsistentCertificate = errors.New("inconsistent server certificate (parsing)")

// ErrProgrammaticBuffer is raised when writing to a dynamic buffer that cannot be expanded.
var ErrProgrammaticBuffer = errors.New("programmatic buffer error")

// Connect opens a connection to a Klio server.
func Connect(logger *slog.Logger, cfg *config.KlioRepositoryClientConfig) (*Connection, error) {
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
	)
	if err != nil {
		return nil, fmt.Errorf("while establishing connection to the server: %w", err)
	}

	walClient := klioGRPC.NewWALClient(conn)

	return &Connection{
		logger:         logger,
		cfg:            cfg,
		walClient:      walClient,
		grpcConnection: conn,
	}, nil
}

// GetWAL get a WAL from a remote connection.
func (c *Connection) GetWAL(ctx context.Context, walName string) (*types.WalEntry, error) {
	client, err := c.walClient.GetWAL(ctx, &klioGRPC.GetWALRequest{
		ClusterName: c.cfg.ClusterName,
		WalName:     walName,
	})
	if err != nil {
		return nil, fmt.Errorf("while starting downloading a WAL file: %w", err)
	}

	var buffer bytes.Buffer

	for {
		result, err := client.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while receiving a WAL file block: %w", err)
		}

		if _, err := buffer.Write(result.GetWalBlock()); err != nil {
			return nil, ErrProgrammaticBuffer
		}
	}

	return types.NewWalEntry(walName, buffer.Bytes()), nil
}

// StoreWAL uploads a WAL in the WAL server
// Important: this function uploads a full WAL file.
func (c *Connection) StoreWAL(ctx context.Context, name string, content []byte) error {
	stream, err := c.walClient.UploadWAL(ctx)
	if err != nil {
		return fmt.Errorf("while starting uploading a WAL file: %w", err)
	}

	walReader := bytes.NewBuffer(content)

	buffer := make([]byte, 4096)
	for {
		readBytes, readError := walReader.Read(buffer)
		if readError != nil && !errors.Is(readError, io.EOF) {
			return status.Errorf(codes.Internal, "error while reading WAL (reading): %v", readError.Error())
		}

		if err := stream.Send(&klioGRPC.UploadWALRequest{
			ClusterName: c.cfg.ClusterName,
			WalName:     name,
			SegmentSize: uint64(len(content)),
			WalBlock:    buffer[:readBytes],
		}); err != nil {
			return status.Errorf(codes.Internal, "error while sending WAL block: %v", err.Error())
		}

		if errors.Is(readError, io.EOF) {
			break
		}
	}

	result, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("while flushing WAL file: %w", err)
	}

	if result.GetWrittenSize() != uint64(len(content)) {
		return &IncompleteWALFileError{
			uploadedSize: result.GetWrittenSize(),
			expectedSize: uint64(len(content)),
		}
	}

	return nil
}

// Close closes the GRPC connection.
func (c *Connection) Close(_ context.Context) error {
	err := c.grpcConnection.Close()
	if err != nil {
		return fmt.Errorf("while closing connection: %w", err)
	}

	return nil
}
