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

package walplayer

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudnative-pg/klio/core/internal/walplayer"
)

// generateCmd is the generate command
//
//nolint:gochecknoglobals
var generateCmd = &cobra.Command{
	Use:   "generate [output-directory]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Generate a directory of WAL files",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDirectory := "."
		if len(args) > 0 {
			targetDirectory = args[0]
		}

		walSizeMB, _ := cmd.Flags().GetInt("wal-size")
		length, _ := cmd.Flags().GetInt("length")

		w, err := walplayer.NewWALWriter(walSizeMB)
		if err != nil {
			return fmt.Errorf("while creating WAL writer: %w", err)
		}

		if err := w.ToDirectory(cmd.Context(), targetDirectory, length); err != nil {
			return fmt.Errorf("writing to %q: %w", targetDirectory, err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	generateCmd.Flags().Int("wal-size", 16, "The WAL file size in MBs. Defaults to 16 MB")

	generateCmd.Flags().Int("length", 0, "How many WAL files should be generated. Required.")
	_ = generateCmd.MarkFlagRequired("length")

	WalPlayerCmd.AddCommand(generateCmd)
}
