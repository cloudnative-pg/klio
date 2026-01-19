package server

import (
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/thejerf/suture/v4"

	"github.com/cloudnative-pg/klio/core/internal/server"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// startCmd represents the start command
//
//nolint:gochecknoglobals
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts a Klio server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig
		contextLogger := log.FromContext(cmd.Context())

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		tier1Enabled, err := cmd.Flags().GetBool("tier1")
		if err != nil {
			return fmt.Errorf("failed to read tier1 flag: %w", err)
		}

		tier2Enabled, err := cmd.Flags().GetBool("tier2")
		if err != nil {
			return fmt.Errorf("failed to read tier2 flag: %w", err)
		}

		if !tier1Enabled && !tier2Enabled {
			return errors.New("at least one of --tier1 or --tier2 must be enabled")
		}

		klio := suture.New("klio", suture.Spec{
			EventHook: func(event suture.Event) {
				contextLogger.Info(event.String())
			},
		})

		// Configure NATS
		var queueURL string
		if tier1Enabled && tier2Enabled {
			if configuration.QueueDirectory == "" {
				return errors.New("queue is required when both tier1 and tier2 are enabled")
			}

			nats, err := server.NewNatsService(configuration.QueueDirectory)
			if err != nil {
				return err
			}

			queueURL = nats.ClientURL()
			klio.Add(nats)
		}

		// Configure tier1
		if tier1Enabled {
			if err := configuration.RequireTier1(); err != nil {
				return fmt.Errorf("tier 1 configuration validation error: %w", err)
			}

			tier1 := suture.NewSimple("tier1")
			tier1.Add(&server.Tier1KopiaServer{
				Config: &configuration,
			})
			tier1.Add(&server.Tier1WALServer{
				Config:   &configuration,
				QueueURL: queueURL,
			})
			klio.Add(tier1)
		}

		// Configure tier2
		if tier2Enabled {
			if err := configuration.RequireTier2(); err != nil {
				return fmt.Errorf("tier 2 configuration validation error: %w", err)
			}

			tier2 := suture.NewSimple("tier2")
			tier2.Add(&server.Tier2KopiaServer{
				Config: &configuration,
			})
			tier2.Add(&server.Tier2WALServer{
				Config: &configuration,
			})
			tier2.Add(&server.Tier2BackupConsumer{
				Config:   &configuration,
				QueueURL: queueURL,
			})
			tier2.Add(&server.Tier2WALConsumer{
				Config:   &configuration,
				QueueURL: queueURL,
			})
			klio.Add(tier2)
		}

		return klio.Serve(cmd.Context())
	},
}

//nolint:gochecknoinits
func init() {
	startCmd.Flags().Bool("tier1", true, "Enables Tier1 server components")
	startCmd.Flags().Bool("tier2", false, "Enables Tier2 server components")

	ServerCmd.AddCommand(startCmd)
}
