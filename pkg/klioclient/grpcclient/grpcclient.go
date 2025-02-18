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
	"google.golang.org/grpc/credentials"

	klioGRPC "github.com/EnterpriseDB/klio/internal/klioserver/grpc"
	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
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

// StoreHistoryFile uses the underlying GRPC connection to store an history file.
func (c *Connection) StoreHistoryFile(ctx context.Context, name string, content []byte) error {
	_, err := c.walClient.UploadHistory(ctx, &klioGRPC.UploadHistoryRequest{
		FileName: name,
		Content:  content,
	})
	if err != nil {
		return fmt.Errorf("while uploading history file %v: %w", name, err)
	}

	return nil
}

// GetWAL get a WAL from a remote connection.
func (c *Connection) GetWAL(ctx context.Context, walName string) (*types.Entry, error) {
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

	return types.NewEntry(walName, buffer.Bytes()), nil
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
			return fmt.Errorf("error while reading WAL (reading from buffer): %w", readError)
		}

		if err := stream.Send(&klioGRPC.UploadWALRequest{
			ClusterName: c.cfg.ClusterName,
			WalName:     name,
			SegmentSize: uint64(len(content)),
			WalBlock:    buffer[:readBytes],
		}); err != nil {
			return fmt.Errorf("error while sending WAL block (sending via GRPC): %w", err)
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

// StoreWALStreaming implements the WAL streaming service.
func (c *Connection) StoreWALStreaming(
	ctx context.Context,
	name string,
	segmentSize uint64,
) (*common.WALUploader, error) {
	stream, err := c.walClient.UploadWAL(ctx)
	if err != nil {
		return nil, fmt.Errorf("while starting uploading a WAL file: %w", err)
	}

	return common.NewWALUploader(&grpcWALStream{
		innerStream: stream,
		segmentSize: segmentSize,
		clusterName: c.cfg.ClusterName,
		walName:     name,
	}), nil
}

type grpcWALStream struct {
	innerStream klioGRPC.WAL_UploadWALClient
	segmentSize uint64
	clusterName string
	walName     string
}

// Close implements common.WALStream.
func (g *grpcWALStream) Close(_ context.Context) error {
	result, err := g.innerStream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("while flushing WAL file: %w", err)
	}

	if result.GetWrittenSize() != g.segmentSize {
		return &IncompleteWALFileError{
			uploadedSize: result.GetWrittenSize(),
			expectedSize: g.segmentSize,
		}
	}

	return nil
}

// SendBlock implements common.WALStream.
func (g *grpcWALStream) SendBlock(_ context.Context, block []byte) error {
	if err := g.innerStream.Send(&klioGRPC.UploadWALRequest{
		ClusterName: g.clusterName,
		WalName:     g.walName,
		SegmentSize: g.segmentSize,
		WalBlock:    block,
	}); err != nil {
		return fmt.Errorf("error while sending WAL block (send streaming len=%v): %w", len(block), err)
	}

	return nil
}
