package server

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	natsServer "github.com/nats-io/nats-server/v2/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// NatsService manages an embedded NATS server with JetStream for task queue operations.
type NatsService struct {
	server *natsServer.Server
}

// NewNatsService creates a new NATS server instance with JetStream enabled in the specified directory.
func NewNatsService(directory string) (*NatsService, error) {
	natsOptions := natsServer.Options{
		JetStream: true,
		// always will make sure servers fsync after every message BEFORE it is acknowledged.
		SyncAlways: true,
		StoreDir:   directory,
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
	defer func() {
		s.server.Shutdown()
		s.server.WaitForShutdown()
	}()

	unregister, err := s.registerQueueMetrics()
	if err != nil {
		return err
	}
	defer func() {
		if err := unregister(); err != nil {
			log.FromContext(ctx).Error(err, "while unregistering NATS queue metrics")
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	return nil
}

func (s *NatsService) String() string {
	return "nats"
}

// registerQueueMetrics registers observable gauges that report the number of
// messages and bytes currently held by JetStream.
func (s *NatsService) registerQueueMetrics() (func() error, error) {
	meter := otel.Meter(opentelemetry.Meter)

	messagesObservableMetric, err := meter.Int64ObservableGauge(
		opentelemetry.QueueMessagesMetric,
		metric.WithDescription("Number of messages currently stored in the embedded NATS JetStream queue."),
		metric.WithUnit("{messages}"),
	)
	if err != nil {
		return nil, fmt.Errorf("while registering %s metric: %w", opentelemetry.QueueMessagesMetric, err)
	}

	bytesObservableMetric, err := meter.Int64ObservableGauge(
		opentelemetry.QueueBytesMetric,
		metric.WithDescription("Number of bytes currently stored in the embedded NATS JetStream queue."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("while registering %s metric: %w", opentelemetry.QueueBytesMetric, err)
	}

	registration, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		info, jszErr := s.server.Jsz(nil)
		if jszErr != nil {
			return jszErr
		}

		//nolint:gosec
		o.ObserveInt64(messagesObservableMetric, int64(info.Messages))
		//nolint:gosec
		o.ObserveInt64(bytesObservableMetric, int64(info.Bytes))

		return nil
	},
		messagesObservableMetric,
		bytesObservableMetric,
	)
	if err != nil {
		return nil, fmt.Errorf("while registering NATS queue metrics callback: %w", err)
	}

	return registration.Unregister, nil
}
