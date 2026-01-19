package server

import (
	"context"

	natsServer "github.com/nats-io/nats-server/v2/server"
)

// NatsService manages an embedded NATS server with JetStream for task queue operations.
type NatsService struct {
	server *natsServer.Server
}

// NewNatsService creates a new NATS server instance with JetStream enabled in the specified directory.
func NewNatsService(directory string) (*NatsService, error) {
	natsOptions := natsServer.Options{
		JetStream: true,
		StoreDir:  directory,
	}

	server, err := natsServer.NewServer(&natsOptions)
	if err != nil {
		return nil, err
	}

	return &NatsService{
		server: server,
	}, nil
}

// ClientURL returns the URL that clients should use to connect to this NATS server.
func (s *NatsService) ClientURL() string {
	return s.server.ClientURL()
}

// Serve starts the NATS server and waits for it to shut down.
func (s *NatsService) Serve(ctx context.Context) error {
	s.server.Start()

	// Wait for context cancellation
	<-ctx.Done()

	// Trigger graceful shutdown
	s.server.Shutdown()
	s.server.WaitForShutdown()

	return nil
}

func (s *NatsService) String() string {
	return "nats"
}
