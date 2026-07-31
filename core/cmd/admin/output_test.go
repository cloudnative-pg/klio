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
	"bytes"
	"strings"
	"testing"
)

func TestWriteTableNoRows(t *testing.T) {
	var buf bytes.Buffer
	err := writeTable(&buf, []string{"ID", "NAME"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	if got != "No results\n" {
		t.Errorf("expected %q, got %q", "No results\n", got)
	}
}

func TestWriteTableEmptyRows(t *testing.T) {
	var buf bytes.Buffer
	err := writeTable(&buf, []string{"ID", "NAME"}, [][]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	if got != "No results\n" {
		t.Errorf("expected %q, got %q", "No results\n", got)
	}
}

func TestWriteTableSingleRow(t *testing.T) {
	var buf bytes.Buffer
	err := writeTable(&buf, []string{"ID", "NAME"}, [][]string{
		{"1", "alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	// tabwriter aligns columns; headers and row must both appear
	if !containsLine(got, "ID") || !containsLine(got, "NAME") {
		t.Errorf("output missing headers, got:\n%s", got)
	}
	if !containsLine(got, "1") || !containsLine(got, "alice") {
		t.Errorf("output missing row data, got:\n%s", got)
	}
}

func TestWriteTableMultipleRows(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"ID", "NAME", "STATUS"}
	rows := [][]string{
		{"1", "alice", "active"},
		{"2", "bob", "inactive"},
		{"42", "carol", "active"},
	}
	err := writeTable(&buf, headers, rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	lines := splitLines(got)
	// 3 rows + 1 header line
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d:\n%s", len(lines), got)
	}
	for _, h := range headers {
		if !containsLine(got, h) {
			t.Errorf("header %q not found in output:\n%s", h, got)
		}
	}
	for _, row := range rows {
		for _, cell := range row {
			if !containsLine(got, cell) {
				t.Errorf("cell %q not found in output:\n%s", cell, got)
			}
		}
	}
}

// splitLines splits s by newline and drops the trailing empty element.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := make([]string, 0)
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	return lines
}

// containsLine reports whether s contains the substring sub on any line.
func containsLine(s, sub string) bool {
	return strings.Contains(s, sub)
}
