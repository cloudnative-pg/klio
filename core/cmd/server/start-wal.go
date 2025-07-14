package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/validator.v2"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver/repository"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// startWALCmd represents the start command
//
//nolint:gochecknoglobals
var startWALCmd = &cobra.Command{
	Use:   "start-wal",
	Short: "Starts a Klio WAL server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		// Configure a listener
		listener, err := net.Listen("tcp", configuration.Wal.ListenAddress)
		if err != nil {
			return fmt.Errorf("cannot listen on TCP socket: %w", err)
		}

		// Load TLS certificates
		cert, err := tls.LoadX509KeyPair(
			configuration.Wal.TLSCert,
			configuration.Wal.TLSKey,
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
			Path:     configuration.Wal.WALPath,
			Password: configuration.Wal.EncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to local repository: %w", err)
		}

		// Starts the WAL server
		opts := []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(tlsConfig)),
		}

		if configuration.Wal.HTPasswdFile != "" {
			decorator, err := walserver.EnsureValidAuthentication(
				configuration.Wal.HTPasswdFile,
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
			walserver.New(log.FromContext(cmd.Context()), repoConnection),
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
