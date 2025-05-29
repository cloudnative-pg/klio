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
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/common"
	klioGRPC "github.com/EnterpriseDB/klio/internal/grpc"
	"github.com/EnterpriseDB/klio/pkg/config"
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
	return c.StoreWAL(ctx, name, content)
}

// GetWALStreaming get a WAL from a remote connection.
func (c *Connection) GetWALStreaming(ctx context.Context, walName string, out io.Writer) error { //nolint:cyclop
	client, err := c.walClient.Get(ctx, &klioGRPC.GetRequest{
		ClusterName: c.cfg.ClusterName,
		WalName:     walName,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return common.ErrMissingWALFile
		}

		return fmt.Errorf("while starting downloading a WAL file: %w", err)
	}

	var expectedSize, writtenBytes int
	for {
		result, err := client.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			if status.Code(err) == codes.NotFound {
				return common.ErrMissingWALFile
			}

			if writtenBytes > 0 {
				return common.IncompleteTransmissionError{
					Inner:        err,
					WrittenBytes: writtenBytes,
				}
			}

			return fmt.Errorf("while receiving a WAL file block: %w", err)
		}

		if expectedSize == 0 {
			expectedSize = int(result.GetSegmentSize()) //nolint:gosec
		}

		b, err := out.Write(result.GetWalBlock())
		if err != nil {
			return fmt.Errorf("while writing WAL file: %w", err)
		}

		writtenBytes += b
	}

	// If this is a partial WAL, we pad it until the target WAL size is reached
	if strings.HasSuffix(walName, ".partial") && expectedSize > writtenBytes {
		if err := c.padWithZeros(out, expectedSize-writtenBytes); err != nil {
			return err
		}
	}

	return nil
}

// StoreWAL uploads a WAL in the WAL server
// Important: this function uploads a full WAL file.
func (c *Connection) StoreWAL(ctx context.Context, name string, content []byte) error {
	stream, err := c.walClient.Put(ctx)
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

		if err := stream.Send(&klioGRPC.PutRequest{
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

// GetLatestWALFile gets the latest WAL file from the repository.
func (c *Connection) GetLatestWALFile(ctx context.Context) (string, error) {
	result, err := c.walClient.GetLatest(ctx, &klioGRPC.GetLatestRequest{
		ClusterName: c.cfg.ClusterName,
	})
	if err != nil {
		return "", fmt.Errorf("while querying for the latest WAL file: %w", err)
	}

	return result.GetWalName(), nil
}

// StoreWALStreaming implements the WAL streaming service.
func (c *Connection) StoreWALStreaming(
	ctx context.Context,
	name string,
	segmentSize uint64,
) (*common.WALUploader, error) {
	stream, err := c.walClient.Put(ctx)
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

func (c *Connection) padWithZeros(wBlocks io.Writer, zeroBytesToWrite int) error {
	blockSize := 1024 * 1024
	zeroBlock := make([]byte, blockSize)

	totalWritten := 0
	for totalWritten < zeroBytesToWrite {
		toWrite := min(blockSize, zeroBytesToWrite-totalWritten)

		n, err := wBlocks.Write(zeroBlock[:toWrite])
		if err != nil {
			return fmt.Errorf("while writing padding zeros to the WAL file: %w", err)
		}

		totalWritten += n
	}

	return nil
}

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

// SendBlock implements common.WALStream.
func (g *grpcWALStream) SendBlock(_ context.Context, block []byte) error {
	if err := g.innerStream.Send(&klioGRPC.PutRequest{
		ClusterName: g.clusterName,
		WalName:     g.walName,
		SegmentSize: g.segmentSize,
		WalBlock:    block,
	}); err != nil {
		return fmt.Errorf("error while sending WAL block (send streaming len=%v): %w", len(block), err)
	}

	g.sentBytes += uint64(len(block))

	return nil
}
