package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/validator.v2"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
	"github.com/cloudnative-pg/klio/core/internal/wal"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// ErrParsingClientCACertificate is raised when we couldn't parse
// the client CA certificate file.
var ErrParsingClientCACertificate = errors.New("parsing client CA certificate file failed")

// startWALCmd represents the start command
//
//nolint:gochecknoglobals
var startWALCmd = &cobra.Command{
	Use:   "start-wal",
	Short: "Starts a Klio WAL server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		logger := log.FromContext(cmd.Context())

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

		// Connects to the Klio repository
		walFS := afero.NewBasePathFs(afero.NewOsFs(), configuration.Wal.WALPath)
		repoConnection, err := repository.Open(repository.Options{
			FS:       walFS,
			Password: configuration.Wal.EncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to local repository: %w", err)
		}

		// Configure TLS
		cert, err := tls.LoadX509KeyPair(
			configuration.Wal.TLSCert,
			configuration.Wal.TLSKey,
		)
		if err != nil {
			return fmt.Errorf("failed to load server key pair: %w", err)
		}

		clientCAPem, err := os.ReadFile(configuration.Wal.ClientCACertFile)
		if err != nil {
			return fmt.Errorf("while reading Client CA certificate file: %w", err)
		}

		clientCAPool := x509.NewCertPool()
		if !clientCAPool.AppendCertsFromPEM(clientCAPem) {
			return ErrParsingClientCACertificate
		}

		// Create TLS configuration
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    clientCAPool,
		}

		// Starts the WAL server
		opts := []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(tlsConfig)),
			grpc.InitialConnWindowSize(256 * 1024),
			grpc.InitialWindowSize(256 * 1024),
			grpc.ReadBufferSize(256 * 1024),
			grpc.WriteBufferSize(256 * 1024),
			grpc.MaxRecvMsgSize(wal.MaxGRPCMessageSizeBytes),
			grpc.MaxSendMsgSize(wal.MaxGRPCMessageSizeBytes),
			grpc.StatsHandler(otelgrpc.NewServerHandler()),
		}

		var queueConnection *queue.Conn
		if configuration.Wal.NATSAddress != "" {
			natsConnection, err := nats.Connect(
				configuration.Wal.NATSAddress,
				nats.RetryOnFailedConnect(true),
				nats.ReconnectWait(1*time.Second),
			)
			if err != nil {
				return fmt.Errorf("error while connecting to the NATS server: %w", err)
			}

			queueConnection = queue.New(natsConnection)
			if err := queueConnection.EnsureSetup(cmd.Context()); err != nil {
				return fmt.Errorf("error while setting up the queue: %w", err)
			}

			logger.Info("NATS server available", "address", configuration.Wal.NATSAddress)
		}

		server := grpc.NewServer(opts...)
		klioGRPC.RegisterWALServer(
			server,
			walserver.New(
				walserver.Options{
					Connection: repoConnection,
					ReadOnly:   false,
					Queue:      queueConnection,
				},
			),
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
