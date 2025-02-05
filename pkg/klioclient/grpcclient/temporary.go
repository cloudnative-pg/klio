package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/EnterpriseDB/klio/internal/klioserver"
	klioGRPC "github.com/EnterpriseDB/klio/internal/klioserver/grpc"
	"github.com/EnterpriseDB/klio/internal/klioserver/repository"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// TemporaryConnection is a connection to a temporary repository, to be
// deleted after the client is closed.
type TemporaryConnection struct {
	Connection

	dirName  string
	listener net.Listener
}

// ConnectTemporary creates a connection to a local Kopia repository, creating it
// if not initialized.
func ConnectTemporary(
	logger *slog.Logger,
	cfg *config.KlioRepositoryClientConfig,
	opts repository.Options,
) (*TemporaryConnection, error) {
	//nolint:gosec
	listeningPort := rand.IntN(1000) + 5000
	address := fmt.Sprintf("localhost:%v", listeningPort)

	if err := repository.Initialize(opts); err != nil {
		return nil, fmt.Errorf("initializing repository: %w", err)
	}

	repoConnection, err := repository.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("cannot open local repository: %w", err)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on TCP socket: %w", err)
	}

	server := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	klioGRPC.RegisterWALServer(
		server,
		klioserver.NewWALServerImplementation(logger, repoConnection),
	)

	go func() {
		if err := server.Serve(listener); !errors.Is(err, net.ErrClosed) {
			slog.Error("error while running temporary server", "err", err)
		}
	}()

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("while establishing connection to the server: %w", err)
	}

	walClient := klioGRPC.NewWALClient(conn)

	return &TemporaryConnection{
		Connection: Connection{
			logger:         logger,
			cfg:            cfg,
			walClient:      walClient,
			grpcConnection: conn,
		},
		listener: listener,
		dirName:  opts.Path,
	}, nil
}

// Close closes the connection to the repository.
func (s *TemporaryConnection) Close(ctx context.Context) error {
	if err := s.listener.Close(); err != nil {
		return fmt.Errorf("while closing listener: %w", err)
	}

	if err := s.Connection.Close(ctx); err != nil {
		return fmt.Errorf("while closing connection: %w", err)
	}

	if err := os.RemoveAll(s.dirName); err != nil {
		return fmt.Errorf("while cleaning up directory %s: %w", s.dirName, err)
	}

	return nil
}
