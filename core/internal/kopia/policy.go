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
	"strconv"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// GetCurrentKopiaPolicy retrieves the current retention policy for a target.
func (s *Client) GetCurrentKopiaPolicy(
	ctx context.Context,
	t Target,
) (*Policy, error) {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"policy",
		"show",
		t.String(),
		"--config-file=" + s.ConfigFile,
		"--disable-file-logging",
		"--json",
	}

	contextLogger.Info("Getting Kopia policy", "args", args, "target", t)

	var buffer bytes.Buffer

	showPolicyCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	showPolicyCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, showPolicyCmd, &buffer); err != nil {
		return nil, fmt.Errorf("error while getting Kopia policy: %w", err)
	}

	var result Policy
	if err := json.NewDecoder(&buffer).Decode(&result); err != nil {
		return nil, fmt.Errorf("cannot decode JSON backup metadata: %w", err)
	}

	return &result, nil
}

// SetKopiaPolicy sets the retention policy for a target.
func (s *Client) SetKopiaPolicy(
	ctx context.Context,
	t Target,
	policy *RetentionPolicy,
) error {
	policyToArgument := func(value *int) string {
		if value == nil {
			return "inherit"
		}

		return strconv.Itoa(*value)
	}

	contextLogger := log.FromContext(ctx)

	args := []string{
		"policy",
		"set",
		"--config-file=" + s.ConfigFile,
		"--disable-file-logging",
		"--keep-annual=" + policyToArgument(policy.KeepAnnual),
		"--keep-daily=" + policyToArgument(policy.KeepDaily),
		"--keep-hourly=" + policyToArgument(policy.KeepHourly),
		"--keep-latest=" + policyToArgument(policy.KeepLatest),
		"--keep-monthly=" + policyToArgument(policy.KeepMonthly),
		"--keep-weekly=" + policyToArgument(policy.KeepWeekly),
		t.String(),
	}

	contextLogger.Info("Setting Kopia policy", "args", args, "target", t)

	setPolicyCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	setPolicyCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, setPolicyCmd, nil); err != nil {
		return fmt.Errorf("error while setting Kopia policy: %w", err)
	}

	return nil
}

// ApplyKopiaPolicy applies the retention policy by expiring old snapshots.
func (s *Client) ApplyKopiaPolicy(ctx context.Context, t Target) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"expire",
		"--config-file=" + s.ConfigFile,
		"--disable-file-logging",
		"--delete",
		t.String(),
	}

	contextLogger.Info("Applying Kopia policy", "args", args, "target", t)

	snapshotExpireCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	snapshotExpireCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, snapshotExpireCmd, nil); err != nil {
		return fmt.Errorf("error while applying Kopia policy: %w", err)
	}

	return nil
}
