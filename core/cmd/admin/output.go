package admin

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// writeTable renders a tab-aligned table with the given headers and rows.
func writeTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(rows) == 0 {
		if _, err := fmt.Fprintln(tw, "No results"); err != nil {
			return err
		}

		return nil
	}

	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}
