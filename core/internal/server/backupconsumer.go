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
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"

	"github.com/cloudnative-pg/klio/core/internal/consumer"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// BackupConsumer processes backup tasks from the queue. It runs whenever tier1
// is enabled and performs the post-backup work for every backup: verification,
// plus the tier2 relay when tier2 is configured. Backup and WAL retention are
// handled independently by the periodic retention sweeper
// (internal/retention, server.RetentionSweeper), not here.
type BackupConsumer struct {
	Config               *config.ServerConfig
	Tier1KopiaConfigFile string
	Tier2KopiaConfigFile string
	QueueURL             string
}

// Serve starts the backup consumer and processes backup tasks from the queue.
func (s *BackupConsumer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("backup-consumer")
	ctx = log.IntoContext(ctx, contextLogger)

	// Connect to NATS
	natsConnection, err := nats.Connect(
		s.QueueURL,
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(1*time.Second),
	)
	if err != nil {
		return fmt.Errorf("error while connecting to the NATS server: %w", err)
	}
	queueConnection, err := queue.New(ctx, natsConnection)
	if err != nil {
		return fmt.Errorf("error while configuring NATS server: %w", err)
	}

	backupOptions := &consumer.BackupOptions{
		Queue:            queueConnection,
		Tier1KopiaConfig: s.Tier1KopiaConfigFile,
		CacheDirectory:   s.Config.Tier1.Base.CacheDirectory,
	}

	// When tier2 is configured, wire the tier2 client so the consumer can
	// relay backups destined for tier2. Without tier2 the consumer only
	// verifies tier1 backups.
	if s.Tier2KopiaConfigFile != "" {
		backupOptions.Tier2KopiaConfig = s.Tier2KopiaConfigFile
	}

	// Starts the consumer
	c, err := consumer.NewBackup(backupOptions)
	if err != nil {
		return fmt.Errorf("error while creating backup consumer: %w", err)
	}

	if err := c.Run(ctx); err != nil {
		return fmt.Errorf("while consuming messages: %w", err)
	}

	return nil
}

func (s *BackupConsumer) String() string {
	return "backup-consumer"
}
