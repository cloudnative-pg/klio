package server

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/k8sapi"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// apiServerCmd represents the job run command
//
//nolint:gochecknoglobals
var apiServerCmd = &cobra.Command{
	Use:    "api-server",
	Short:  "Starts the Klio API aggregation server",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the default values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Client == (config.ClientConfig{}) {
			return cli.ErrClientSectionIsRequired
		}
		if configuration.Client.Base == (config.BaseRepositoryClientConfig{}) {
			return cli.ErrKopiaClientSectionIsRequired
		}

		if errs := validator.Validate(&configuration.Client.Base); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		connection, err := kopia.Connect(
			cmd.Context(),
			&configuration.Client.Base,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w %q", err, configuration.Client.Base.URL)
		}

		return k8sapi.Start(cmd.Context(), connection)
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(apiServerCmd)
}
