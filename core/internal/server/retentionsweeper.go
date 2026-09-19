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
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/retention"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// retentionSweepInterval is how often the RetentionSweeper re-evaluates
// retention for every cluster. Fixed for now (no config/CRD knob).
const retentionSweepInterval = time.Minute

// RetentionSweeper periodically lists every cluster's backups and deletes
// whatever is out of retention, for tier1 and (when configured) tier2. It
// runs whenever tier1 is enabled, alongside BackupConsumer.
type RetentionSweeper struct {
	Config               *config.ServerConfig
	Tier1KopiaConfigFile string
	Tier2KopiaConfigFile string
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

	sweeper, err := retention.NewSweeper(sweeperOptions)
	if err != nil {
		return fmt.Errorf("error while creating retention sweeper: %w", err)
	}

	ticker := time.NewTicker(retentionSweepInterval)
	defer ticker.Stop()

	// Run once immediately so a fresh server doesn't wait a full interval
	// before its first sweep.
	if err := sweeper.Sweep(ctx); err != nil {
		contextLogger.Error(err, "Error while sweeping retention")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := sweeper.Sweep(ctx); err != nil {
				contextLogger.Error(err, "Error while sweeping retention")
			}
		}
	}
}

func (s *RetentionSweeper) String() string {
	return "retention-sweeper"
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

	// Extract the certificate fingerprint for the Kopia servers. Tier1 and
	// tier2 share the same server certificate.
	certificateFingerprint, err := kopia.ExtractSHA256CertificateFingerprint(
		s.Config.TLS.TLSCert)
	if err != nil {
		return nil, fmt.Errorf("error while extracting fingerprint of the kopia server certificate: %w", err)
	}

	sweeperOptions := &retention.Options{
		Tier1KopiaConfig:                  s.Tier1KopiaConfigFile,
		Tier1ServerAddress:                "https://" + s.Config.Tier1.Base.ListenAddress,
		Tier1ServerCertificateFingerprint: certificateFingerprint,
		RunID:                             s.RunID,
		RunSecret:                         s.RunSecret,
		Tier1WALRepository:                tier1WALRepository,
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
		sweeperOptions.Tier2ServerCertificateFingerprint = certificateFingerprint
		sweeperOptions.Tier2WALRepository = tier2WALRepository
	}

	return sweeperOptions, nil
}
