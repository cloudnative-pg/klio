package tier1

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
	"github.com/thejerf/suture/v4"

	"github.com/EnterpriseDB/klio/internal/infrastructure"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// Service is the tier1 specification.
type Service interface {
	suture.Service

	// GetLatestWALFileName returns the latest WAL file name
	GetLatestWALFileName(ctx context.Context) (string, error)

	// GetWAL returns the content of a WAL file
	GetWAL(ctx context.Context, walName string) (*WalEntry, error)

	// StoreWAL stores a WAL file inside the repository
	StoreWAL(ctx context.Context, name string, content []byte) error

	// IsReady returns true if the underlying storage is ready
	IsReady() bool
}

type impl struct {
	config         *config.Data
	logger         *slog.Logger
	repository     repo.Repository
	infrastructure infrastructure.Service
	segmentSize    uint64
}

func (s *impl) IsReady() bool {
	return s != nil && s.repository != nil
}

// New creates a new tier1 service.
func New(cfg *config.Data, log *slog.Logger, infra infrastructure.Service) Service { //nolint:ireturn
	return &impl{
		config:         cfg,
		logger:         log.With("service", "tier1"),
		infrastructure: infra,
	}
}

// String implements the stringer interface.
func (*impl) String() string {
	return "tier1"
}

// Serve implements the service interface.
//
//nolint:cyclop
func (s *impl) Serve(ctx context.Context) error {
	fsStorage, err := filesystem.New(
		ctx,
		&filesystem.Options{
			Path: path.Join(s.config.Tier1.Path, "data"),
		},
		true,
	)
	if err != nil {
		return fmt.Errorf("while creating storage interface: %w", err)
	}

	// Ensures the current Kopia client is connected to the repository and the configuration
	// file is persisted
	configFile := path.Join(s.config.Tier1.Path, "kopiacfg")
	if err := repo.Connect(ctx, configFile, fsStorage, s.config.Tier1.Password, &repo.ConnectOptions{
		ClientOptions: repo.ClientOptions{
			Hostname: s.config.ClusterName,
		},
	}); err != nil {
		if errors.Is(err, repo.ErrRepositoryNotInitialized) {
			s.logger.Info("repository is not initialized, triggering initialization")
			// Triggering repository initialization
			if err := repo.Initialize(ctx, fsStorage, &repo.NewRepositoryOptions{}, s.config.Tier1.Password); err != nil {
				return fmt.Errorf("while initializing repository: %w", err)
			}
		} else {
			return fmt.Errorf("while connecting to repository: %w", err)
		}
	}

	segmentSize, err := s.infrastructure.GetWalSegmentSize(ctx)
	if err != nil {
		return fmt.Errorf("while getting wal segment size: %w", err)
	}

	s.segmentSize = segmentSize

	// Opens a connection to the repository using the persisted configuration file
	repository, err := repo.Open(ctx, configFile, s.config.Tier1.Password, &repo.Options{})
	if err != nil {
		return fmt.Errorf("while opening the repository: %w", err)
	}

	// make the repository available only if there are no errors
	s.repository = repository

	defer func() {
		if err := s.repository.Close(ctx); err != nil {
			s.logger.Error("while closing the repository", "err", err)
		}
		s.repository = nil
		s.segmentSize = 0
	}()

	cmd, err := s.generateServerStartCommand(ctx)
	if err != nil {
		return fmt.Errorf("while generating the server start command: %w", err)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == -1 {
			// Process was killed by a signal, do not return an error
			s.logger.Warn("Kopia server process was killed by a signal", "output", string(output))
		} else {
			return fmt.Errorf("while starting the Kopia server, output '%s', error code: %w", output, err)
		}
	}

	return nil
}

func (s *impl) generateServerStartCommand(ctx context.Context) (*exec.Cmd, error) {
	s.logger.Debug("generating command", "serverConf", s.config.Server)

	commandContent := []string{
		"server", "start",
		"--tls-cert-file", s.config.Server.TLSCertPath,
		"--tls-key-file", s.config.Server.TLSKeyPath,
		"--address", fmt.Sprintf("0.0.0.0:%d", s.config.Server.Port),
		"--server-control-username", s.config.ClusterName,
	}

	// https://kopia.io/docs/repository-server/
	// Note that when starting the server again the --tls-generate-cert must be omitted,
	// otherwise the server will fail to start.
	if s.config.Server.GenerateCertificates {
		// add the flag to generate the certificates if we check that they both don't already exist
		var certError, keyError error
		_, certError = os.Stat(s.config.Server.TLSCertPath)
		_, keyError = os.Stat(s.config.Server.TLSKeyPath)

		if os.IsNotExist(certError) && os.IsNotExist(keyError) {
			s.logger.Info("adding the flag to generate the certificates on server start")
			commandContent = append(commandContent, "--tls-generate-cert")
		} else if certError != nil || keyError != nil {
			return nil, fmt.Errorf("while checking the certificates, cert: '%w', key: %w", certError, keyError)
		}
	}

	cmd := exec.CommandContext(ctx, "kopia", commandContent...) //nolint:gosec

	// Set environment variables
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("KOPIA_PASSWORD=%s", s.config.Tier1.Password),
		fmt.Sprintf("KOPIA_SERVER_CONTROL_PASSWORD=%s", s.config.Tier1.Password),
		fmt.Sprintf("KOPIA_CONFIG_PATH=%s", "/data/kopiacfg"),
	)

	return cmd, nil
}
