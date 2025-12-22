package retention

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// setCmd represents the retention get command
//
//nolint:gochecknoglobals
var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Sets the currently applied retention policy",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Client == (config.ClientConfig{}) {
			return cli.ErrClientSectionIsRequired
		}
		if configuration.Client.Base == (config.BaseRepositoryClientConfig{}) {
			return cli.ErrKopiaClientSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		client, err := kopia.MultiConnect(
			cmd.Context(),
			&configuration.Client.Base,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w %q", err, configuration.Client.Base.URL)
		}

		target := klioclient.Target{
			Hostname: client.GetHostname(),
			Username: client.GetUsername(),
		}

		effectivePolicy, err := client.GetRetentionPolicy(cmd.Context(), target)
		if err != nil {
			return fmt.Errorf("while getting the current retention policy: %w", err)
		}

		getKeepValue := func(name string) *int {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				return nil
			}

			if !f.Changed {
				return nil
			}

			value, err := strconv.Atoi(f.Value.String())
			if err != nil {
				return nil
			}

			return &value
		}

		if effectivePolicy == nil {
			effectivePolicy = &klioclient.RetentionPolicy{}
		}
		effectivePolicy.KeepLatest = getKeepValue("keep-latest")
		effectivePolicy.KeepAnnual = getKeepValue("keep-annual")
		effectivePolicy.KeepMonthly = getKeepValue("keep-monthly")
		effectivePolicy.KeepWeekly = getKeepValue("keep-weekly")
		effectivePolicy.KeepDaily = getKeepValue("keep-daily")
		effectivePolicy.KeepHourly = getKeepValue("keep-hourly")

		if err := client.SetRetentionPolicy(cmd.Context(), target, *effectivePolicy); err != nil {
			return fmt.Errorf("while setting the current retention policy: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	// The following flags are really misleading.
	// We should find a way to better document them.
	setCmd.Flags().Int("keep-latest", 0, "Number of most recent latest backup kept")
	setCmd.Flags().Int("keep-annual", 0, "Number of most recent annual backup kept")
	setCmd.Flags().Int("keep-monthly", 0, "Number of most recent monthly backup kept")
	setCmd.Flags().Int("keep-weekly", 0, "Number of most recent weekly backup kept")
	setCmd.Flags().Int("keep-daily", 0, "Number of most recent daily backup kept")
	setCmd.Flags().Int("keep-hourly", 0, "Number of most recent hourly backup kept")

	RetentionCmd.AddCommand(setCmd)
}
