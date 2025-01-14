package tier1

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
	"github.com/thejerf/suture/v4"

	"github.com/EnterpriseDB/klio/pkg/config"
)

// Service is the tier1 specification
type Service interface {
	suture.Service

	// GetLatestWALFile retu
	GetLatestWALFile() (string, error)

	// StoreWAL stores a WAL file inside the repository
	StoreWAL(ctx context.Context, name string, content []byte) error
}

type impl struct {
	config     *config.Data
	logger     *slog.Logger
	repository repo.Repository
}

// New creates a new tier1 service
func New(cfg *config.Data, log *slog.Logger) Service {
	return &impl{
		config: cfg,
		logger: log.With("service", "tier1"),
	}
}

// String implements the stringer interface
func (*impl) String() string {
	return "tier1"
}

// GetLatestWALFile returns the latest WAL file that have been archived
func (*impl) GetLatestWALFile() (string, error) {
	panic("not implemented")
}

// Serve implements the service interface
func (s *impl) Serve(ctx context.Context) error {
	st, err := filesystem.New(
		ctx,
		&filesystem.Options{
			Path: path.Join(s.config.Tier1.Path, "data"),
		},
		true,
	)
	if err != nil {
		return fmt.Errorf("while creating storage interface: %w", err)
	}

	configFile := path.Join(s.config.Tier1.Path, "kopiacfg")
	if err := repo.Connect(ctx, configFile, st, s.config.Tier1.Password, &repo.ConnectOptions{
		ClientOptions: repo.ClientOptions{
			Hostname: s.config.ClusterName,
		},
	}); err != nil {
		if errors.Is(err, repo.ErrRepositoryNotInitialized) {
			s.logger.Info("repository is not initialized, triggering initialization")
			if err := repo.Initialize(ctx, st, &repo.NewRepositoryOptions{}, s.config.Tier1.Password); err != nil {
				return fmt.Errorf("while initializing repository: %w", err)
			}
		} else {
			return fmt.Errorf("while connecting to repository: %w", err)
		}
	}

	s.repository, err = repo.Open(ctx, configFile, s.config.Tier1.Password, &repo.Options{})
	if err != nil {
		return fmt.Errorf("while opening the repository: %w", err)
	}

	defer func() {
		if err := s.repository.Close(ctx); err != nil {
			s.logger.Error("while closing the repository", "err", err)
		}
		s.repository = nil
	}()

	<-ctx.Done()

	return nil
}
