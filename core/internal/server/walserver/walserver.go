package walserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/wal"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Start starts a WAL server.
func Start(
	ctx context.Context,
	repoConnection *repository.Connection,
	walServerConfiguration *config.WalServerConfig,
	tlsConfiguration *config.TLSConfig,
	queueURL string,
) error {
	logger := log.FromContext(ctx)

	// Configure a listener
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", walServerConfiguration.ListenAddress)
	if err != nil {
		return fmt.Errorf("cannot listen on TCP socket: %w", err)
	}

	// Configure TLS
	cert, err := tls.LoadX509KeyPair(
		tlsConfiguration.TLSCert,
		tlsConfiguration.TLSKey,
	)
	if err != nil {
		return fmt.Errorf("failed to load server key pair: %w", err)
	}

	clientCAPem, err := os.ReadFile(tlsConfiguration.ClientCACertFile)
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
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
	}

	// Starts the WAL server
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.InitialConnWindowSize(wal.GRPCInitialConnWindowSizeBytes),
		grpc.InitialWindowSize(wal.GRPCInitialWindowSizeBytes),
		grpc.ReadBufferSize(wal.GRPCSocketBufferSizeBytes),
		grpc.WriteBufferSize(wal.GRPCSocketBufferSizeBytes),
		grpc.MaxRecvMsgSize(wal.MaxGRPCMessageSizeBytes),
		grpc.MaxSendMsgSize(wal.MaxGRPCMessageSizeBytes),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	var queueConnection *queue.Conn
	if queueURL != "" {
		natsConnection, err := nats.Connect(
			queueURL,
			nats.RetryOnFailedConnect(true),
			nats.ReconnectWait(1*time.Second),
		)
		if err != nil {
			return fmt.Errorf("error while connecting to the NATS server: %w", err)
		}

		if queueConnection, err = queue.New(ctx, natsConnection); err != nil {
			return fmt.Errorf("error while setting up the queue: %w", err)
		}

		logger.Info("NATS server available", "address", queueURL)
	}

	server := grpc.NewServer(opts...)
	klioGRPC.RegisterWALServer(
		server,
		New(
			Options{
				Connection: repoConnection,
				ReadOnly:   false,
				Queue:      queueConnection,
			},
		),
	)

	go func() {
		// Wait for context cancellation
		<-ctx.Done()

		// Trigger graceful shutdown
		server.GracefulStop()
	}()

	if err := server.Serve(listener); !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("error while running server: %w", err)
	}

	return nil
}
