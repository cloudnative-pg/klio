package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	kopiaWrapper "github.com/cloudnative-pg/klio/core/internal/kopia"
)

// Options contains configuration options for the admin server.
type Options struct {
	Tier1KopiaConfigFile string
	Tier2KopiaConfigFile string

	SocketPath string

	RunID                  string
	RunSecret              string
	CertificateFingerprint string

	Tier2ServerAddress string
}

// Server is the gRPC admin server implementation.
type Server struct {
	klioGRPC.UnimplementedAdminServer

	opts Options

	tier1KopiaClient *kopiaWrapper.Client
	tier2KopiaClient *kopiaWrapper.Client
	klio             klioclient.Client
}

// New creates a new admin server instance with the provided options.
func New(opts Options) (*Server, error) {
	kopiaBinary, err := kopiaWrapper.LookupBinary()
	if err != nil {
		return nil, err
	}

	tier1, err := clientFromConfigFile(opts.Tier1KopiaConfigFile)
	if err != nil {
		return nil, err
	}

	tier2, err := clientFromConfigFile(opts.Tier2KopiaConfigFile)
	if err != nil {
		return nil, err
	}

	return &Server{
		klio: &kopia.MultiConnection{
			Tier1: tier1,
			Tier2: tier2,
		},
		tier1KopiaClient: &kopiaWrapper.Client{
			ConfigFile:  opts.Tier1KopiaConfigFile,
			KopiaBinary: kopiaBinary,
			Password:    "",
		},
		tier2KopiaClient: &kopiaWrapper.Client{
			ConfigFile:  opts.Tier2KopiaConfigFile,
			KopiaBinary: kopiaBinary,
			Password:    "",
		},
		opts: opts,
	}, nil
}

// Start starts the admin server and listens for gRPC connections.
func (s *Server) Start(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	// Remove any stale socket file
	if err := os.Remove(s.opts.SocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("while removing stale socket file: %w", err)
	}

	// Create the Unix socket
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "unix", s.opts.SocketPath)
	if err != nil {
		return err
	}

	// Set restrictive permissions on the socket file (owner read/write only).
	// This ensures only the user running the server can connect to the admin socket.
	if err := os.Chmod(s.opts.SocketPath, 0o600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("while setting socket permissions: %w", err)
	}

	server := grpc.NewServer()
	klioGRPC.RegisterAdminServer(server, s)

	go func() {
		// Wait for context cancellation
		<-ctx.Done()

		// Trigger graceful shutdown
		server.GracefulStop()
	}()

	contextLogger.Info("Starting Klio administration server", "socketPath", s.opts.SocketPath)
	if err := server.Serve(listener); !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("error while running server: %w", err)
	}

	return nil
}

func clientFromConfigFile(configFile string) (klioclient.Client, error) {
	if configFile == "" {
		return nil, nil
	}

	return kopia.FromKopiaConfig(configFile)
}

// ListBackups implements [grpc.AdminServer].
func (s *Server) ListBackups(
	ctx context.Context,
	_ *klioGRPC.ListBackupsRequest,
) (*klioGRPC.ListBackupsResult, error) {
	backupList, err := s.klio.ListBackups(ctx, "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "while listing backups: %s", err.Error())
	}

	data, err := json.Marshal(backupList)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "while marshalling backup manifest: %s", err.Error())
	}

	return &klioGRPC.ListBackupsResult{
		BackupManifests: data,
	}, nil
}

// Refresh implements [grpc.AdminServer].
func (s *Server) Refresh(
	ctx context.Context,
	_ *klioGRPC.RefreshRequest,
) (*klioGRPC.RefreshResult, error) {
	if s.tier2KopiaClient != nil {
		if err := s.tier2KopiaClient.RefreshServer(ctx, kopiaWrapper.RefreshServerOptions{
			ServerControlUser:     s.opts.RunID,
			ServerControlPassword: s.opts.RunSecret,
			ServerCertFingerprint: s.opts.CertificateFingerprint,
			Address:               s.opts.Tier2ServerAddress,
		}); err != nil {
			return nil, status.Errorf(codes.Internal, "while refreshing tier2 server: %s", err.Error())
		}
	}

	return &klioGRPC.RefreshResult{}, nil
}
