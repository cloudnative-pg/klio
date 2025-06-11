package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/validator.v2"

	klioGRPC "github.com/EnterpriseDB/klio/internal/grpc"
	"github.com/EnterpriseDB/klio/internal/server/walserver"
	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// startWALCmd represents the start command
//
//nolint:gochecknoglobals
var startWALCmd = &cobra.Command{
	Use:   "start-wal",
	Short: "Starts a Klio WAL server",
	RunE: func(_ *cobra.Command, _ []string) error {
		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Server == nil {
			return ErrKlioServerSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		// Configure a listener
		listener, err := net.Listen("tcp", configuration.Server.Wal.ListenAddress)
		if err != nil {
			return fmt.Errorf("cannot listen on TCP socket: %w", err)
		}

		// Load TLS certificates
		cert, err := tls.LoadX509KeyPair(
			configuration.Server.Wal.TLSCert,
			configuration.Server.Wal.TLSKey,
		)
		if err != nil {
			return fmt.Errorf("failed to load server key pair: %w", err)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		}

		// Connects to the Klio repository
		repoConnection, err := repository.Open(repository.Options{
			Path:     configuration.Server.Wal.WALPath,
			Password: configuration.Server.Wal.EncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to local repository: %w", err)
		}

		// Starts the WAL server
		opts := []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(tlsConfig)),
		}

		if configuration.Server.Wal.HTPasswdFile != "" {
			decorator, err := walserver.EnsureValidAuthentication(
				configuration.Server.Wal.HTPasswdFile,
			)
			if err != nil {
				return fmt.Errorf("while initializing htpasswd file parser: %w", err)
			}

			opts = append(
				opts,
				grpc.UnaryInterceptor(decorator),
			)
		}

		server := grpc.NewServer(opts...)
		klioGRPC.RegisterWALServer(
			server,
			walserver.New(slog.Default(), repoConnection),
		)
		if err := server.Serve(listener); !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("error while running server: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(startWALCmd)
}
