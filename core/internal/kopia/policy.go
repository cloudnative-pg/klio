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

// SetKopiaCompressionPolicy sets the compression policy for a source. This
// overrides the repository-wide global policy for that source.
func (s *Client) SetKopiaCompressionPolicy(
	ctx context.Context,
	t Target,
	policy CompressionPolicy,
) error {
	return s.setKopiaCompressionPolicy(ctx, t.String(), policy)
}

// SetKopiaGlobalCompressionPolicy sets the repository-wide (global)
// compression policy, which applies to every source that does not define its
// own compression policy.
func (s *Client) SetKopiaGlobalCompressionPolicy(
	ctx context.Context,
	policy CompressionPolicy,
) error {
	return s.setKopiaCompressionPolicy(ctx, "--global", policy)
}

// setKopiaCompressionPolicy runs `kopia policy set` with the compression flags
// against the passed policy target (a "user@host" source or the "--global"
// selector).
func (s *Client) setKopiaCompressionPolicy(
	ctx context.Context,
	policyTarget string,
	policy CompressionPolicy,
) error {
	contextLogger := log.FromContext(ctx)

	args := buildCompressionPolicyArgs(s.ConfigFile, policyTarget, policy)

	contextLogger.Info("Setting Kopia compression policy", "args", args)

	setPolicyCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	setPolicyCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, setPolicyCmd, nil); err != nil {
		return fmt.Errorf("error while setting Kopia compression policy: %w", err)
	}

	return nil
}

// buildCompressionPolicyArgs builds the argument list for the
// `kopia policy set` compression command. policyTarget is either a
// "user@host" source or the "--global" selector.
//
// An unset algorithm emits no flag, leaving whatever the repository already
// holds. The size bounds, in contrast, are always emitted: a zero value is
// sent as "inherit", which Kopia resets to its inherited default. Skipping
// them instead would make a bound impossible to clear once written, because
// no value of the configuration field could ever remove it.
func buildCompressionPolicyArgs(configFile, policyTarget string, policy CompressionPolicy) []string {
	args := []string{
		"policy",
		"set",
		"--config-file=" + configFile,
		// Kopia's own on-disk log files are suppressed on every invocation:
		// Klio captures the subprocess output via RunWithLogCapture and
		// re-emits it as structured logs, so the files would only accumulate
		// redundantly on the cache volume.
		"--disable-file-logging",
	}

	if policy.Algorithm != "" {
		args = append(args, "--compression="+policy.Algorithm)
	}
	args = append(args,
		"--compression-min-size="+compressionSizeArg(policy.MinSize),
		"--compression-max-size="+compressionSizeArg(policy.MaxSize),
	)

	return append(args, policyTarget)
}

// kopiaInheritPolicyValue is the value Kopia's `policy set` accepts to reset a
// field to the value inherited from its parent policy.
const kopiaInheritPolicyValue = "inherit"

// compressionSizeArg renders a compression size bound as a `kopia policy set`
// flag value, mapping the zero value to "inherit" so that clearing the bound in
// the configuration also clears it in the repository.
func compressionSizeArg(size int64) string {
	if size <= 0 {
		return kopiaInheritPolicyValue
	}

	return strconv.FormatInt(size, 10)
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
