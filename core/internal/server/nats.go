/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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
// messages and bytes currently held by each JetStream stream. Values carry a
// `stream` attribute identifying the source stream (e.g. the WAL work queue,
// the backup work queue, and the latest-uploaded-WAL retention stream).
func (s *NatsService) registerQueueMetrics() (func() error, error) {
	meter := otel.Meter(opentelemetry.Meter)

	messagesObservableMetric, err := meter.Int64ObservableGauge(
		opentelemetry.ServerQueueMessagesMetric,
		metric.WithDescription("Number of messages currently stored in each JetStream stream. "+
			"The `stream` attribute identifies the source stream."),
		metric.WithUnit("{messages}"),
	)
	if err != nil {
		return nil, fmt.Errorf("while registering %s metric: %w", opentelemetry.ServerQueueMessagesMetric, err)
	}

	bytesObservableMetric, err := meter.Int64ObservableGauge(
		opentelemetry.ServerQueueBytesMetric,
		metric.WithDescription("Number of bytes currently stored in each JetStream stream. "+
			"The `stream` attribute identifies the source stream."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("while registering %s metric: %w", opentelemetry.ServerQueueBytesMetric, err)
	}

	registration, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		info, jszErr := s.server.Jsz(&natsServer.JSzOptions{Streams: true})
		if jszErr != nil {
			return jszErr
		}

		for _, account := range info.AccountDetails {
			for _, stream := range account.Streams {
				attrs := metric.WithAttributes(opentelemetry.AttributeKeyStream.Of(stream.Name))
				//nolint:gosec
				o.ObserveInt64(messagesObservableMetric, int64(stream.State.Msgs), attrs)
				//nolint:gosec
				o.ObserveInt64(bytesObservableMetric, int64(stream.State.Bytes), attrs)
			}
		}

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
