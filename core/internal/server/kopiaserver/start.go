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

package kopiaserver

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Config contains the information required to start up a Kopia server.
type Config struct {
	// ListenAddress is the <host>:<port> specification that is
	// used to create the listening socket.
	ListenAddress string

	// Kopia server control username. This credential can be used
	// to access the REST control API.
	ServerControlUser string

	// Kopia server control password. This credential can be used
	// to access the REST control API.
	ServerControlPassword string
}

// start runs a Kopia server with the passed configuration. The tier value
// labels the snapshot metrics emitted by the collector so callers can tell
// tier-1 and tier-2 series apart.
func start(
	ctx context.Context, configFile string, cfg *Config, tls *config.TLSConfig, tier opentelemetry.Tier,
) error {
	contextLogger := log.FromContext(ctx)

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	// Create observable uptime metric
	serverStartTime := time.Now()
	_, err = otel.Meter(opentelemetry.Meter).Float64ObservableGauge(
		opentelemetry.ServerUptimeMetric,
		metric.WithDescription("Klio server uptime in seconds."),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(time.Since(serverStartTime).Seconds())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("while creating uptime metric: %w", err)
	}

	// Start the snapshot metrics collector
	metricsCollector, err := newSnapshotMetricsCollector(
		configFile,
		time.Minute,
		tier,
		contextLogger)
	if err != nil {
		return fmt.Errorf("while creating snapshot metrics collector: %w", err)
	}

	go metricsCollector.Start(ctx)
	defer metricsCollector.Stop()

	wrapper := kopia.Client{
		KopiaBinary: kopiaBinary,
		ConfigFile:  configFile,
	}

	if err := wrapper.RunServer(ctx, kopia.ServerOptions{
		TLSCert:               tls.TLSCert,
		TLSKey:                tls.TLSKey,
		ClientCACertFile:      tls.ClientCACertFile,
		ListenAddress:         cfg.ListenAddress,
		ServerControlUser:     cfg.ServerControlUser,
		ServerControlPassword: cfg.ServerControlPassword,
	}); err != nil {
		return fmt.Errorf("while starting the kopia server: %w", err)
	}

	return nil
}
