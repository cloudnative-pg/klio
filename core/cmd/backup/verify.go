package backup

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// corruptionExitCode is the exit code used when backup verification
// detects actual corruption (errorCount > 0). Infrastructure errors
// use the default exit code 1.
const corruptionExitCode = 2

// tierSelection represents which tiers to verify.
type tierSelection struct {
	Tier1 bool
	Tier2 bool
}

// parseTiers parses the --tiers flag value into a tierSelection.
func parseTiers(tiersFlag string) tierSelection {
	result := tierSelection{}
	for tier := range strings.SplitSeq(tiersFlag, ",") {
		switch strings.TrimSpace(tier) {
		case "tier1":
			result.Tier1 = true
		case "tier2":
			result.Tier2 = true
		}
	}

	return result
}

// verifyCmd represents the klio backup verify command.
//
//nolint:gochecknoglobals
var verifyCmd = &cobra.Command{
	Use:   "verify [backup-names...]",
	Short: "Verify the integrity of backups",
	Long: `Verify the integrity of backups in the repository.

By default, verifies only the backup names passed as arguments.
Use --all to verify all backups in the repository.
Use --tiers to select which tiers to verify (default: both).`,
	Args: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if !all && len(args) == 0 {
			return errors.New("requires at least one backup name, or use --all")
		}
		if all && len(args) > 0 {
			return errors.New("cannot specify both --all and backup names")
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runVerify(cmd, args)
		if err != nil {
			var backupErr *kopia.BackupVerificationError
			if errors.As(err, &backupErr) {
				// Print the error and exit with corruption exit code.
				// At this point, all defers in runVerify have already executed.
				cmd.PrintErrln(err)
				os.Exit(corruptionExitCode)
			}
		}

		return err
	},
}

func runVerify(cmd *cobra.Command, args []string) error {
	configuration, err := loadAndValidateConfig()
	if err != nil {
		return err
	}

	// Parse flags
	all, _ := cmd.Flags().GetBool("all")
	tiersFlag, _ := cmd.Flags().GetString("tiers")
	tiers := parseTiers(tiersFlag)

	// Connect to tier1
	tier1Client, err := kopia.ConnectTier1(
		cmd.Context(),
		&configuration.Client,
	)
	if err != nil {
		return fmt.Errorf("while connecting to tier1: %w %q", err, configuration.Client.Base.URL)
	}
	defer tier1Client.Close(cmd.Context())

	// Get hostname from connection
	hostname := tier1Client.GetHostname()

	// Build verify options
	opts := kopia.VerifyOpts{
		Hostname:    hostname,
		All:         all,
		BackupNames: args,
	}

	// Verify tier1
	if tiers.Tier1 {
		if err := verifyTier(cmd, tier1Client, opts, "tier1"); err != nil {
			return err
		}
	}

	// Verify tier2 if configured and requested
	if tiers.Tier2 && configuration.Client.Base.Tier2URL != "" {
		tier2Client, err := kopia.ConnectTier2(
			cmd.Context(),
			&configuration.Client,
		)
		if err != nil {
			return fmt.Errorf("while connecting to tier2: %w %q", err, configuration.Client.Base.Tier2URL)
		}
		defer tier2Client.Close(cmd.Context())

		if err := verifyTier(cmd, tier2Client, opts, "tier2"); err != nil {
			return err
		}
	}

	return nil
}

func loadAndValidateConfig() (*config.Data, error) {
	var configuration config.Data

	// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
	// when using environment variables
	if err := viper.Unmarshal(&configuration); err != nil {
		return nil, fmt.Errorf("could not unmarshal configuration: %w", err)
	}

	// Sets the default values, to be overridden by the user configuration
	configuration.SetDefaults()

	if configuration.Client == (config.ClientConfig{}) {
		return nil, cli.ErrClientSectionIsRequired
	}
	if configuration.Client.Base == (config.BaseRepositoryClientConfig{}) {
		return nil, cli.ErrKopiaClientSectionIsRequired
	}

	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation error: %w", err)
	}

	return &configuration, nil
}

func verifyTier(cmd *cobra.Command, client *kopia.Connection, opts kopia.VerifyOpts, tierName string) error {
	contextLogger := log.FromContext(cmd.Context())
	contextLogger.Info("Verifying backups", "tier", tierName)

	if err := client.VerifyBackups(cmd.Context(), opts); err != nil {
		var backupErr *kopia.BackupVerificationError
		if errors.As(err, &backupErr) {
			contextLogger.Error(err, "Backup verification detected corruption", "tier", tierName)
			return err
		}

		return fmt.Errorf("%s backup verification failed: %w", tierName, err)
	}

	contextLogger.Info("Backup verification completed successfully", "tier", tierName)

	return nil
}

//nolint:gochecknoinits
func init() {
	verifyCmd.Flags().Bool("all", false, "Verify all backups instead of specific names")
	verifyCmd.Flags().String("tiers", "tier1,tier2", "Tiers to verify (tier1, tier2, or tier1,tier2)")
	BackupCmd.AddCommand(verifyCmd)
}
