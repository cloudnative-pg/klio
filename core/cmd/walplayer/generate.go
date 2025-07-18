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

		w := walplayer.NewWALWriter(walSizeMB)
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
