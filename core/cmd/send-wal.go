package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/client/sendwal"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// ErrTimeoutWaitingPG is raised when we couldn't get a connection to PostgreSQL.
var ErrTimeoutWaitingPG = errors.New("timeout waiting for PostgreSQL connection")

// runCmd represents the run command
//
//nolint:gochecknoglobals
var sendWalCmd = &cobra.Command{
	Use:   "send-wal",
	Short: "Upload the cluster's WALs to the target Klio server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := log.FromContext(cmd.Context())

		// Create a context that gets cancelled on SIGINT/SIGTERM
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Source == (config.SourceConfig{}) {
			return cli.ErrSourceSectionIsRequired
		}

		if configuration.Client == (config.ClientConfig{}) {
			return cli.ErrClientSectionIsRequired
		}

		if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
			return cli.ErrKlioClientSectionIsRequired
		}

		if err := configuration.Validate(); err != nil {
			return fmt.Errorf("configuration validation error: %w", err)
		}

		waitForPrimary, _ := cmd.Flags().GetBool("primary")
		if !retryWaitForPostgreSQLInstance(ctx, configuration.Source.StandardDSN, waitForPrimary, time.Hour) {
			return ErrTimeoutWaitingPG
		}

		client, err := grpcclient.Connect(&configuration.Client.Wal, configuration.Client.Wal.Address)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		sendToTier2, _ := cmd.Flags().GetBool("enable-tier2-backup")

		err = sendwal.New(&configuration, logger, client, sendToTier2).
			Start(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logger.Info("send-wal stopped due to context cancellation, exiting gracefully")
			return nil
		}

		return err
	},
}

func retryWaitForPostgreSQLInstance(ctx context.Context, dsn string, waitForPrimary bool, timeout time.Duration) bool {
	const probeInterval = 1 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			return false
		}

		if !waitForPostgreSQLInstance(ctx, dsn, waitForPrimary) {
			time.Sleep(probeInterval)
			continue
		}

		break
	}

	return true
}

func waitForPostgreSQLInstance(ctx context.Context, dsn string, waitForPrimary bool) bool {
	logger := log.FromContext(ctx)

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		logger.Info("PostgreSQL not available, retrying", "err", err)
		return false
	}

	defer func() {
		if err := conn.Close(ctx); err != nil {
			logger.Info("Error while closing PostgreSQL connection", "err", err)
		}
	}()

	if waitForPrimary {
		var isInRecovery bool
		row := conn.QueryRow(ctx, "SELECT pg_is_in_recovery()", pgx.QueryExecModeSimpleProtocol)
		if err := row.Scan(&isInRecovery); err != nil {
			logger.Info("Cannot detect if the PostgreSQL instance is a primary or a replica", "err", err)
			return false
		} else if isInRecovery {
			logger.Info("Replica detected, waiting", "primaryDsn", isInRecovery)
			return false
		}

		return true
	}

	return true
}

//nolint:gochecknoinits
func init() {
	sendWalCmd.Flags().Bool(
		"primary", true, "Wait for the current instance to become a primary")

	sendWalCmd.Flags().Bool("enable-tier2-backup", false, "Send WALs to tier2")

	rootCmd.AddCommand(sendWalCmd)
}
