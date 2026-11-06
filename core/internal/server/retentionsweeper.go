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
	"errors"
	"fmt"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/retention"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// retentionSweepInterval is how often the RetentionSweeper re-evaluates
// retention for every cluster. Fixed for now (no config/CRD knob).
const retentionSweepInterval = time.Minute

// errQueueConnectionClosed is returned when the queue connection is closed
// for good (it stopped reconnecting), so the supervisor restarts the sweeper
// with a new one.
var errQueueConnectionClosed = errors.New("queue connection closed")

// RetentionSweeper periodically lists every cluster's backups and deletes
// whatever is out of retention, for tier1 and (when configured) tier2. It
// runs whenever tier1 is enabled, alongside BackupConsumer.
type RetentionSweeper struct {
	Config               *config.ServerConfig
	Tier1KopiaConfigFile string
	Tier2KopiaConfigFile string
	QueueURL             string
	RunID                string
	RunSecret            string
}

// Serve builds a retention.Sweeper and runs it on a fixed interval until
// ctx is done.
func (s *RetentionSweeper) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("retention-sweeper")
	ctx = log.IntoContext(ctx, contextLogger)

	sweeperOptions, err := s.buildSweeperOptions(ctx)
	if err != nil {
		return err
	}

	// With tier2 configured, the WAL relay queue tells which WALs have not
	// reached tier2 yet, so tier1 WAL retention keeps them.
	var queueClosed <-chan struct{}
	if s.Tier2KopiaConfigFile != "" {
		natsConnection, closed, err := s.connectPendingWALs(ctx, sweeperOptions)
		if err != nil {
			return err
		}
		defer natsConnection.Close()
		queueClosed = closed
	}

	sweeper, err := retention.NewSweeper(sweeperOptions)
	if err != nil {
		return fmt.Errorf("error while creating retention sweeper: %w", err)
	}

	return runSweeps(ctx, sweeper, queueClosed)
}

func (s *RetentionSweeper) String() string {
	return "retention-sweeper"
}

// connectPendingWALs connects to the queue and sets it as the source of the
// WALs pending relay to tier2 in opts. The caller closes the returned
// connection. The returned channel is closed once the connection is closed for
// good (it stopped reconnecting).
func (s *RetentionSweeper) connectPendingWALs(
	ctx context.Context,
	opts *retention.Options,
) (*nats.Conn, <-chan struct{}, error) {
	closed := make(chan struct{})
	natsConnection, err := nats.Connect(
		s.QueueURL,
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(1*time.Second),
		nats.ClosedHandler(func(*nats.Conn) { close(closed) }),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error while connecting to the NATS server: %w", err)
	}

	// Configure the queue like the other services do, so the WAL stream
	// exists (and any legacy stream is drained into it) before the first
	// sweep reads it.
	queueConnection, err := queue.New(ctx, natsConnection)
	if err != nil {
		natsConnection.Close()
		return nil, nil, fmt.Errorf("error while configuring NATS server: %w", err)
	}
	opts.PendingWALs = queueConnection

	return natsConnection, closed, nil
}

// buildSweeperOptions connects to the tier1 WAL repository (where the
// per-cluster retention policy is stored) and, when tier2 is configured,
// the tier2 WAL repository too, wiring both into a retention.Options.
func (s *RetentionSweeper) buildSweeperOptions(ctx context.Context) (*retention.Options, error) {
	tier1WALFS := afero.NewBasePathFs(afero.NewOsFs(), s.Config.Tier1.Wal.WALPath)
	tier1WALRepository, err := repository.Open(repository.Options{
		FS:       tier1WALFS,
		Password: s.Config.Tier1.EncryptionKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open tier1 WAL repository: %w", err)
	}

	sweeperOptions := &retention.Options{
		Tier1KopiaConfig:   s.Tier1KopiaConfigFile,
		Tier1ServerAddress: "https://" + s.Config.Tier1.Base.ListenAddress,
		ServerTLSCertFile:  s.Config.TLS.TLSCert,
		RunID:              s.RunID,
		RunSecret:          s.RunSecret,
		Tier1WALRepository: tier1WALRepository,
	}

	// When tier2 is configured, wire it so the sweep also prunes tier2
	// backups and WAL. Without tier2 the sweep only prunes tier1.
	if s.Tier2KopiaConfigFile != "" {
		tier2WALFS, err := tier2.ConnectWAL(ctx, &s.Config.Tier2)
		if err != nil {
			return nil, fmt.Errorf("error while connecting to tier2 WAL storage: %w", err)
		}
		tier2WALRepository, err := repository.Open(repository.Options{
			FS:       tier2WALFS,
			Password: s.Config.Tier2.EncryptionKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to open tier2 WAL repository: %w", err)
		}

		sweeperOptions.Tier2KopiaConfig = s.Tier2KopiaConfigFile
		sweeperOptions.Tier2ServerAddress = "https://" + s.Config.Tier2.BaseListenAddress
		sweeperOptions.Tier2WALRepository = tier2WALRepository
	}

	return sweeperOptions, nil
}

// runSweeps runs sweeper once immediately and then on a fixed interval until
// ctx is done, or until queueClosed is closed. queueClosed is nil when the
// sweeper doesn't use the queue.
func runSweeps(ctx context.Context, sweeper *retention.Sweeper, queueClosed <-chan struct{}) error {
	contextLogger := log.FromContext(ctx)

	ticker := time.NewTicker(retentionSweepInterval)
	defer ticker.Stop()

	// Run once immediately so a fresh server doesn't wait a full interval
	// before its first sweep.
	if err := sweeper.Sweep(ctx); err != nil {
		contextLogger.Error(err, "Retention sweep finished with errors")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-queueClosed:
			// Every sweep would fail its tier1 WAL retention without the
			// queue, so let the supervisor restart the sweeper instead.
			return errQueueConnectionClosed
		case <-ticker.C:
			if err := sweeper.Sweep(ctx); err != nil {
				contextLogger.Error(err, "Retention sweep finished with errors")
			}
		}
	}
}
