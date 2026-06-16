package cli

import (
	"errors"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
)

// RunEWithExitCode wraps a cobra RunE so that an error implementing
// ExitCoder terminates the process with its classification exit code,
// letting the parent CNPG sidecar classify the failure. The wrapped
// function returns before os.Exit is called, so all of its defers run
// first. Errors that do not carry an exit code are returned unchanged
// for cobra's default handling (printed with an "Error:" prefix, exit 1).
//
// Every backup subcommand must route its RunE through this wrapper:
// otherwise a repository failure surfaces as the default exit code and
// the parent classifies it as `unknown` instead of its real category.
func RunEWithExitCode(
	fn func(cmd *cobra.Command, args []string) error,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := fn(cmd, args)

		if coded, ok := errors.AsType[ExitCoder](err); ok {
			// Emit the failure through the structured logger so it is
			// logged as JSON, consistent with the rest of Klio's output,
			// rather than printed as plain text. fn has already returned,
			// so all of its defers have run.
			log.FromContext(cmd.Context()).Error(err, "command failed", "command", cmd.CommandPath())
			os.Exit(coded.ExitCode())
		}

		return err
	}
}
