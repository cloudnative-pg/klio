package server

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/thejerf/suture/v4"

	"github.com/cloudnative-pg/klio/core/internal/server"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaconfig"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

type serverOpts struct {
	tier1 bool
	tier2 bool

	cfg             *config.ServerConfig
	adminSocketPath string
	runID           string
	runSecret       string
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

	var tier1ConfigFileName, tier2ConfigFileName string

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

		if err := kopiaconfig.CreateTier1KopiaConfigFile(
			ctx,
			tier1ConfigFileName,
			&opts.cfg.Tier1,
		); err != nil {
			return fmt.Errorf("error creating tier1 kopia config file: %w", err)
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

		tier2ConfigFile, err := os.CreateTemp("", "kopiaconfig_tier2_*")
		if err != nil {
			return fmt.Errorf("while writing a temporary Kopia config: %w", err)
		}

		tier2ConfigFileName = tier2ConfigFile.Name()

		defer func() {
			if err := os.Remove(tier2ConfigFileName); err != nil {
				contextLogger.Warning(
					"Error while removing temporary Tier 2 opts.cfg file",
					"err", err,
					"configFile", tier2ConfigFileName,
				)
			}
		}()

		// When tier1 is disabled, the server operates in read-only mode.
		// The CRD validation enforces that mode: read-only requires no tier1 and
		// must have tier2, so !tier1 is equivalent to read-only mode.
		// Read-only is enforced at the repository connection level via
		// `kopia repository connect --readonly`.
		tier2ReadOnly := !opts.tier1
		if err := kopiaconfig.CreateTier2KopiaConfigFile(
			ctx,
			tier2ConfigFileName,
			&opts.cfg.Tier2,
			tier2ReadOnly,
		); err != nil {
			return fmt.Errorf("error creating tier2 kopia config file: %w", err)
		}

		tier2 := suture.NewSimple("tier2")
		tier2.Add(&server.Tier2KopiaServer{
			Config:      opts.cfg,
			KopiaConfig: tier2ConfigFileName,
			RunID:       opts.runID,
			RunSecret:   opts.runSecret,
		})
		tier2.Add(&server.Tier2WALServer{
			Config: opts.cfg,
		})
		klio.Add(tier2)
	}

	if opts.tier1 && opts.tier2 {
		// We can skip the validation here:
		// both tier1 and tier2 have been already validated, if existing
		relay := suture.NewSimple("relay")
		relay.Add(&server.Tier2BackupConsumer{
			Config:               opts.cfg,
			Tier1KopiaConfigFile: tier1ConfigFileName,
			Tier2KopiaConfigFile: tier2ConfigFileName,
			QueueURL:             queueURL,
			RunID:                opts.runID,
			RunSecret:            opts.runSecret,
		})
		relay.Add(&server.Tier2WALConsumer{
			Config:   opts.cfg,
			QueueURL: queueURL,
		})
		klio.Add(relay)
	}

	// Configure administration server
	adminServer := server.AdminServer{
		Tier1KopiaConfigFile: tier1ConfigFileName,
		Tier2KopiaConfigFile: tier2ConfigFileName,
		SocketPath:           opts.adminSocketPath,
		Config:               opts.cfg,
		RunID:                opts.runID,
		RunSecret:            opts.runSecret,
		QueueURL:             queueURL,
	}
	klio.Add(&adminServer)

	return klio.Serve(ctx)
}
