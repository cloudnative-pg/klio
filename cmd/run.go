// Package cmd is the implementation of the "run" command
package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/thejerf/suture/v4"
	"github.com/thejerf/sutureslog"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/internal/receiver"
	"github.com/EnterpriseDB/klio/internal/tier1"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := slog.Default()

		var configuration config.Data

		// Sets the the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return err
		}

		logger.Info("Current Klio configuration", "configuration", configuration)

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		supervisor := suture.New(
			"klio",
			suture.Spec{
				EventHook: (&sutureslog.Handler{
					Logger: logger,
				}).MustHook(),
			},
		)

		tier1Service := tier1.New(&configuration, logger)
		supervisor.Add(tier1Service)
		supervisor.Add(receiver.New(&configuration, logger, tier1Service))

		return supervisor.Serve(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
