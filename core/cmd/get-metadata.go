package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/grpcclient"
	klioGRPC "github.com/EnterpriseDB/klio/internal/grpc"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// getMetadataCmd represents the get-metadata command
//
//nolint:gochecknoglobals
var getMetadataCmd = &cobra.Command{
	Use:   "get-metadata",
	Short: "Get the metadata of a cluster from the target Klio server",
	Args:  cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := slog.Default()

		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Client == (config.ClientConfig{}) {
			return ErrClientSectionIsRequired
		}

		if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
			return ErrKlioClientSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		client, err := grpcclient.Connect(
			logger,
			&configuration.Client.Wal,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		metadata, err := client.GetMetadata(cmd.Context(), &klioGRPC.GetMetadataRequest{
			ClusterName: configuration.Client.Wal.ClusterName,
		})
		if err != nil {
			return fmt.Errorf("while downloading metadata: %w", err)
		}

		_, err = os.Stdout.WriteString(protojson.Format(metadata))
		if err != nil {
			return fmt.Errorf("while downloading metadata: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(getMetadataCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
