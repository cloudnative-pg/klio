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
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/thejerf/suture/v4"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/server"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaconfig"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// applyGlobalCompressionPolicy sets the repository-wide (global) Kopia
// compression policy using the passed persistent config file. It is a no-op
// when the policy carries no settings. This runs before the Kopia servers
// start, so the direct write to the repository predates any server cache.
func applyGlobalCompressionPolicy(
	ctx context.Context,
	configFile string,
	compression config.CompressionServerConfig,
) error {
	if compression.IsZero() {
		return nil
	}

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	client := &kopia.Client{
		KopiaBinary: kopiaBinary,
		ConfigFile:  configFile,
	}

	return client.SetKopiaGlobalCompressionPolicy(ctx, kopia.CompressionPolicy{
		Algorithm: compression.Algorithm,
		MinSize:   compression.MinSize,
		MaxSize:   compression.MaxSize,
	})
}

// setupTier1KopiaConfig connects the tier1 config file to the repository and
// applies the tier1 repository-wide compression policy.
func setupTier1KopiaConfig(ctx context.Context, configFile string, cfg *config.Tier1Config) error {
	if err := kopiaconfig.CreateTier1KopiaConfigFile(ctx, configFile, cfg); err != nil {
		return fmt.Errorf("error creating tier1 kopia config file: %w", err)
	}

	if err := applyGlobalCompressionPolicy(ctx, configFile, cfg.Compression); err != nil {
		return fmt.Errorf("error setting tier1 global compression policy: %w", err)
	}

	return nil
}

type serverOpts struct {
	tier1 bool
	tier2 bool

	cfg             *config.ServerConfig
	adminSocketPath string
	runID           string
	runSecret       string
}

type tier2ConfigFiles struct {
	rwConfigFileName string
	roConfigFileName string
}

// setupTier2ConfigFiles creates the RW and RO config files for tier2.
// It returns a cleanup function that should be deferred by the caller.
func setupTier2ConfigFiles(
	ctx context.Context,
	cfg *config.Tier2Config,
) (*tier2ConfigFiles, func(), error) {
	contextLogger := log.FromContext(ctx)

	// We need two tier2 repository configurations: one allowing writes and
	// a different one disallowing them.
	//
	// The read-only configuration file is used for the Kopia server and
	// will prevent clients from writing to the repository.
	//
	// The read-write configuration file is used to migrate new snapshots
	// to tier2 and to invoke the maintenance.

	tier2RWConfigFile, err := os.CreateTemp("", "kopiaconfig_tier2_rw_*")
	if err != nil {
		return nil, nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	rwConfigFileName := tier2RWConfigFile.Name()
	if err := tier2RWConfigFile.Close(); err != nil {
		contextLogger.Warning(
			"Error while closing temporary Tier 2 RW config file",
			"err", err,
			"configFile", rwConfigFileName,
		)
	}

	tier2ROConfigFile, err := os.CreateTemp("", "kopiaconfig_tier2_ro_*")
	if err != nil {
		// Clean up the RW file we already created.
		_ = os.Remove(rwConfigFileName)
		return nil, nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	roConfigFileName := tier2ROConfigFile.Name()
	if err := tier2ROConfigFile.Close(); err != nil {
		contextLogger.Warning(
			"Error while closing temporary Tier 2 RO config file",
			"err", err,
			"configFile", roConfigFileName,
		)
	}

	cleanup := func() {
		if err := os.Remove(rwConfigFileName); err != nil {
			contextLogger.Warning(
				"Error while removing temporary Tier 2 RW config file",
				"err", err,
				"configFile", rwConfigFileName,
			)
		}
		if err := os.Remove(roConfigFileName); err != nil {
			contextLogger.Warning(
				"Error while removing temporary Tier 2 RO config file",
				"err", err,
				"configFile", roConfigFileName,
			)
		}
	}

	if err := kopiaconfig.CreateTier2KopiaConfigFile(
		ctx,
		rwConfigFileName,
		cfg,
		false,
	); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("error creating tier2 RW kopia config file: %w", err)
	}

	if err := kopiaconfig.CreateTier2KopiaConfigFile(
		ctx,
		roConfigFileName,
		cfg,
		true,
	); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("error creating tier2 RO kopia config file: %w", err)
	}

	return &tier2ConfigFiles{
		rwConfigFileName: rwConfigFileName,
		roConfigFileName: roConfigFileName,
	}, cleanup, nil
}

//nolint:cyclop
func runServer(ctx context.Context, opts serverOpts) error {
	contextLogger := log.FromContext(ctx)

	klio := suture.New("klio", suture.Spec{
		EventHook: func(event suture.Event) {
			contextLogger.Info(event.String())
		},
	})

	// Configure NATS
	// The queue is always required when tier1 is enabled to support retention policy
	// enforcement. Retention needs to check for pending tier2 transfers even when
	// tier2 is not configured, to allow consistent behavior across configurations.
	var queueURL string
	if opts.tier1 {
		if opts.cfg.QueueDirectory == "" {
			return errors.New("queue is required when tier1 is enabled")
		}

		nats, err := server.NewNatsService(opts.cfg.QueueDirectory)
		if err != nil {
			return err
		}

		queueURL = nats.ClientURL()
		klio.Add(nats)
	}

	var tier1ConfigFileName, tier2RWConfigFileName, tier2ROConfigFileName string

	// Configure tier1
	if opts.tier1 {
		if err := opts.cfg.RequireTier1(); err != nil {
			return fmt.Errorf("tier 1 opts.cfg validation error: %w", err)
		}

		tier1ConfigFile, err := os.CreateTemp("", "kopiaconfig_tier1_*")
		if err != nil {
			return fmt.Errorf("while writing a temporary Kopia config: %w", err)
		}

		tier1ConfigFileName = tier1ConfigFile.Name()

		defer func() {
			if err := os.Remove(tier1ConfigFileName); err != nil {
				contextLogger.Warning(
					"Error while removing temporary Tier 1 opts.cfg file",
					"err", err,
					"configFile", tier1ConfigFileName,
				)
			}
		}()

		if err := setupTier1KopiaConfig(ctx, tier1ConfigFileName, &opts.cfg.Tier1); err != nil {
			return err
		}

		tier1 := suture.NewSimple("tier1")
		tier1.Add(&server.Tier1KopiaServer{
			Config:      opts.cfg,
			KopiaConfig: tier1ConfigFileName,
			RunID:       opts.runID,
			RunSecret:   opts.runSecret,
		})
		tier1.Add(&server.Tier1WALServer{
			Config:   opts.cfg,
			QueueURL: queueURL,
		})
		klio.Add(tier1)
	}

	// Configure tier2
	if opts.tier2 {
		if err := opts.cfg.RequireTier2(); err != nil {
			return fmt.Errorf("tier 2 opts.cfg validation error: %w", err)
		}

		tier2Configs, tier2Cleanup, err := setupTier2ConfigFiles(ctx, &opts.cfg.Tier2)
		if err != nil {
			return err
		}
		defer tier2Cleanup()

		tier2RWConfigFileName = tier2Configs.rwConfigFileName
		tier2ROConfigFileName = tier2Configs.roConfigFileName

		if err := applyGlobalCompressionPolicy(
			ctx, tier2RWConfigFileName, opts.cfg.Tier2.Compression,
		); err != nil {
			return fmt.Errorf("error setting tier2 global compression policy: %w", err)
		}

		tier2 := suture.NewSimple("tier2")
		tier2.Add(&server.Tier2KopiaServer{
			Config:      opts.cfg,
			KopiaConfig: tier2ROConfigFileName,
			RunID:       opts.runID,
			RunSecret:   opts.runSecret,
		})
		tier2.Add(&server.Tier2WALServer{
			Config: opts.cfg,
		})
		klio.Add(tier2)
	}

	// The backup consumer performs the post-backup processing (tier1
	// maintenance for every backup and, when the backup is destined for
	// tier2, the tier2 relay). It runs whenever tier1 is enabled. The
	// tier2 relay is gated per-backup on the request, so when tier2 is not
	// configured Tier2KopiaConfigFile is empty and the consumer only runs
	// tier1 maintenance.
	if opts.tier1 {
		// We can skip the validation here: tier1 (and tier2, if enabled)
		// have already been validated above.
		postBackup := suture.NewSimple("postbackup")
		postBackup.Add(&server.BackupConsumer{
			Config:               opts.cfg,
			Tier1KopiaConfigFile: tier1ConfigFileName,
			Tier2KopiaConfigFile: tier2RWConfigFileName,
			QueueURL:             queueURL,
			RunID:                opts.runID,
			RunSecret:            opts.runSecret,
		})

		// The WAL relay to tier2 only makes sense when tier2 is configured.
		if opts.tier2 {
			postBackup.Add(&server.Tier2WALConsumer{
				Config:   opts.cfg,
				QueueURL: queueURL,
			})
		}
		klio.Add(postBackup)
	}

	// Configure administration server
	adminServer := server.AdminServer{
		Tier1KopiaConfigFile: tier1ConfigFileName,
		Tier2KopiaConfigFile: tier2RWConfigFileName,
		SocketPath:           opts.adminSocketPath,
		Config:               opts.cfg,
		RunID:                opts.runID,
		RunSecret:            opts.runSecret,
		QueueURL:             queueURL,
	}
	klio.Add(&adminServer)

	return klio.Serve(ctx)
}
