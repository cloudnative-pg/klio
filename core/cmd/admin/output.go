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
