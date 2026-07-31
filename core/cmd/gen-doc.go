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

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// categoryConfig represents the _category_.json structure.
type categoryConfig struct {
	Label       string `json:"label"`
	Position    int    `json:"position"`
	Collapsed   bool   `json:"collapsed"`
	Collapsible bool   `json:"collapsible"`
}

// genDocCmd represents the gen-doc command
//
//nolint:gochecknoglobals
var genDocCmd = &cobra.Command{
	Use:    "gen-doc [wal-name] [target-file]",
	Short:  "CLI documentation generator for Klio commands",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Disable the auto-generated tag footer
		rootCmd.DisableAutoGenTag = true

		// Do not document "klio completion" because its syntax is not
		// compatible with Docusaurus.
		commands := rootCmd.Commands()
		if i := slices.IndexFunc(commands, func(c *cobra.Command) bool { return c.Name() == "completion" }); i != -1 {
			commands[i].Hidden = true
		}

		outputDir, err := cmd.Flags().GetString("output")
		if err != nil {
			return fmt.Errorf("while getting output flag: %w", err)
		}

		identity := func(s string) string { return s }

		// Create output directory
		//nolint:gosec // Documentation directory needs to be readable by web server
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		// Generate markdown with custom frontmatter
		if err := doc.GenMarkdownTreeCustom(rootCmd, outputDir, frontmatterFunc, identity); err != nil {
			return fmt.Errorf("generating markdown: %w", err)
		}

		// Create _category_.json
		if err := createCategoryFile(outputDir); err != nil {
			return fmt.Errorf("creating category file: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(genDocCmd)

	genDocCmd.Flags().String("output", ".", "output directory for generated documentation (default: current directory)")
}

// frontmatterFunc generates YAML frontmatter for each command.
func frontmatterFunc(filename string) string {
	// Extract command name from filename (remove .md extension)
	name := strings.TrimSuffix(filepath.Base(filename), ".md")

	// Generate title from command name (replace underscores with spaces)
	title := strings.ReplaceAll(name, "_", " ")

	return fmt.Sprintf(`---
title: %s
---

`, title)
}

// createCategoryFile creates the _category_.json file for Docusaurus.
func createCategoryFile(outputDir string) error {
	config := categoryConfig{
		Label:       "CLI Reference",
		Position:    150,
		Collapsed:   true,
		Collapsible: true,
	}

	filePath := filepath.Join(outputDir, "_category_.json")
	f, err := os.Create(filepath.Clean(filePath))
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("marshaling category config: %w", err)
	}

	return nil
}
