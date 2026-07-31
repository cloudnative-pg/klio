/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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
