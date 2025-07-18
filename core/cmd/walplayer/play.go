package walplayer

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/cmd/clierrors"
	"github.com/cloudnative-pg/klio/core/internal/walplayer"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// playCmd is the play command
//
//nolint:gochecknoglobals
var playCmd = &cobra.Command{
	Use:   "play [directory]",
	Args:  cobra.ExactArgs(1),
	Short: "Send to Klio a directory of WAL files",
	RunE: func(cmd *cobra.Command, args []string) error {
		// cobra.ExactArgs makes sure that there is exactly one argument
		targetDirectory := args[0]

		workers, _ := cmd.Flags().GetInt("jobs")
		blockSize, _ := cmd.Flags().GetInt("block-size")

		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Client == (config.ClientConfig{}) {
			return clierrors.ErrClientSectionIsRequired
		}

		if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
			return clierrors.ErrKlioClientSectionIsRequired
		}

		if errs := validator.Validate(&configuration.Client.Wal); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		cfg := walplayer.NewPlayer(workers, targetDirectory, blockSize*1024, &configuration.Client.Wal)

		results := cfg.Play(cmd.Context())

		encoder := json.NewEncoder(cmd.OutOrStdout())
		for _, result := range results {
			if err := encoder.Encode(result); err != nil {
				return fmt.Errorf("could not marshal result: %w", err)
			}
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	playCmd.Flags().IntP("jobs", "j", 1, "Number of parallel jobs to use when sending WALs")
	playCmd.Flags().Int("block-size", 2048, "Block size in KB. Defaults to 2048.")
	WalPlayerCmd.AddCommand(playCmd)
}
