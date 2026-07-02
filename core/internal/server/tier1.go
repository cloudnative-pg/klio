package server

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Tier1WALServer manages the tier 1 WAL server component.
type Tier1WALServer struct {
	Config   *config.ServerConfig
	QueueURL string
}

// Serve starts the tier 1 WAL server and handles incoming requests.
func (s *Tier1WALServer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("tier1-grpc")
	ctx = log.IntoContext(ctx, contextLogger)

	// Connects to the Klio repository
	walFS := afero.NewBasePathFs(afero.NewOsFs(), s.Config.Tier1.Wal.WALPath)
	repoConnection, err := repository.Open(repository.Options{
		FS:       walFS,
		Password: s.Config.Tier1.EncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to local repository: %w", err)
	}

	if err := walserver.Start(
		ctx,
		repoConnection,
		&s.Config.Tier1.Wal,
		&s.Config.TLS,
		s.QueueURL,
		opentelemetry.Tier1,
	); err != nil {
		return fmt.Errorf("while starting the WAL server: %w", err)
	}

	return nil
}

func (s *Tier1WALServer) String() string {
	return "tier1-grpc"
}

// Tier1KopiaServer manages the tier 1 Kopia server component.
type Tier1KopiaServer struct {
	Config      *config.ServerConfig
	KopiaConfig string
	RunID       string
	RunSecret   string
}

// Serve starts the tier 1 Kopia server and handles backup operations.
func (s *Tier1KopiaServer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("tier1-kopia")
	ctx = log.IntoContext(ctx, contextLogger)

	if err := kopiaserver.StartTier1(
		ctx,
		s.Config.Tier1.Base.ListenAddress,
		&s.Config.TLS,
		kopiaserver.ServerControlCredential{
			User:     s.RunID,
			Password: s.RunSecret,
		},
		s.KopiaConfig,
	); err != nil {
		return fmt.Errorf("while running kopia server: %w", err)
	}

	return nil
}

func (s *Tier1KopiaServer) String() string {
	return "tier1-kopia"
}
