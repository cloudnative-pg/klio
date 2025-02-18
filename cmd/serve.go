// Package cmd is the implementation of the "run" command
package cmd

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

// serveCmd represents the serve command
//
//nolint:gochecknoglobals
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts a Klio server",
	RunE: func(_ *cobra.Command, _ []string) error {
		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.KlioServerConfig == nil {
			return ErrKlioServerSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		// Configure a listener
		listener, err := net.Listen("tcp", configuration.KlioServerConfig.ListenAddress)
		if err != nil {
			return fmt.Errorf("cannot listen on TCP socket: %w", err)
		}

		// Load TLS certificates
		cert, err := tls.LoadX509KeyPair(
			configuration.KlioServerConfig.ServerCertPath,
			configuration.KlioServerConfig.ServerKeyPath,
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
			Path:     configuration.KlioServerConfig.WALPath,
			Password: configuration.KlioServerConfig.Password,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to local repository: %w", err)
		}

		// Starts the WAL server
		server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
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
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
