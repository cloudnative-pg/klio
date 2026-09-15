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

package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// DisableKopiaRetention turns off Kopia's own snapshot retention in the
// repository, since Klio applies retention itself. `kopia repository create`
// stores Kopia's default retention counters in the global policy and every
// `kopia snapshot create` expires snapshots against the effective policy,
// so without this Kopia keeps deleting backups behind Klio's back.
//
// An all-zero retention policy is Kopia's encoding for "keep everything";
// the per-source policies left by earlier Klio versions are reset to inherit
// the global one. It must run before the Kopia server starts, so the direct
// writes predate any server cache.
func (s *Client) DisableKopiaRetention(ctx context.Context) error {
	if err := s.setPolicyRetention(ctx, "--global", "0"); err != nil {
		return fmt.Errorf("while disabling global Kopia retention: %w", err)
	}

	targets, err := s.listPolicyTargets(ctx)
	if err != nil {
		return err
	}

	for _, target := range targets {
		if target.IsGlobal() {
			continue
		}

		if err := s.setPolicyRetention(ctx, target.String(), "inherit"); err != nil {
			return fmt.Errorf("while resetting Kopia retention of %q: %w", target.String(), err)
		}
	}

	return nil
}

func (s *Client) setPolicyRetention(ctx context.Context, policyTarget string, value string) error {
	args := buildRetentionPolicyArgs(s.ConfigFile, policyTarget, value)

	log.FromContext(ctx).Info("Setting Kopia retention policy", "args", args)

	setPolicyCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	setPolicyCmd.Env = s.kopiaEnvironmentVariables()

	return RunWithLogCapture(ctx, setPolicyCmd, nil)
}

func buildRetentionPolicyArgs(configFile, policyTarget, value string) []string {
	args := []string{
		"policy",
		"set",
		"--config-file=" + configFile,
		"--disable-file-logging",
	}

	for _, counter := range kopiaRetentionCounters() {
		args = append(args, counter+"="+value)
	}

	return append(args, policyTarget)
}

// kopiaRetentionCounters are the retention flags of `kopia policy set`.
func kopiaRetentionCounters() []string {
	return []string{
		"--keep-latest",
		"--keep-hourly",
		"--keep-daily",
		"--keep-weekly",
		"--keep-monthly",
		"--keep-annual",
	}
}

func (s *Client) listPolicyTargets(ctx context.Context) ([]SourceInfo, error) {
	listCmd := exec.CommandContext(ctx, s.KopiaBinary, //nolint:gosec
		"policy", "list", "--json", "--config-file="+s.ConfigFile, "--disable-file-logging")
	listCmd.Env = s.kopiaEnvironmentVariables()

	var stdout bytes.Buffer
	if err := RunWithLogCapture(ctx, listCmd, &stdout); err != nil {
		return nil, fmt.Errorf("while listing Kopia policies: %w", err)
	}

	return parsePolicyTargets(stdout.Bytes())
}

func parsePolicyTargets(raw []byte) ([]SourceInfo, error) {
	var policies []struct {
		Target SourceInfo `json:"target"`
	}
	if err := json.Unmarshal(raw, &policies); err != nil {
		return nil, fmt.Errorf("while unmarshalling Kopia policy list %q: %w", string(raw), err)
	}

	targets := make([]SourceInfo, len(policies))
	for i := range policies {
		targets[i] = policies[i].Target
	}

	return targets, nil
}
