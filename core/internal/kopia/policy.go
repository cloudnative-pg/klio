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
	"cmp"
	"context"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

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

// DisableKopiaGlobalRetentionPolicy sets every Kopia keep-* retention field
// to 0 on the repository-wide (global) policy. Kopia treats "all six keep-*
// fields explicitly zero" as its own sentinel for "keep everything".
// klio's own retention sweeper is the sole retention authority; Kopia's
// built-in policy-based expiry must never independently delete a snapshot.
func (s *Client) DisableKopiaGlobalRetentionPolicy(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	args := buildDisableRetentionPolicyArgs(s.ConfigFile)

	contextLogger.Info("Disabling Kopia's built-in global retention policy", "args", args)

	setPolicyCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	setPolicyCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, setPolicyCmd, nil); err != nil {
		return fmt.Errorf("error while disabling Kopia's global retention policy: %w", err)
	}

	return nil
}

// buildDisableRetentionPolicyArgs builds the argument list for the
// `kopia policy set --global` command that zeroes every keep-* field.
func buildDisableRetentionPolicyArgs(configFile string) []string {
	return []string{
		"policy",
		"set",
		"--config-file=" + configFile,
		"--disable-file-logging",
		"--keep-latest=0",
		"--keep-hourly=0",
		"--keep-daily=0",
		"--keep-weekly=0",
		"--keep-monthly=0",
		"--keep-annual=0",
		"--global",
	}
}

// buildCompressionPolicyArgs builds the argument list for the
// `kopia policy set` compression command. policyTarget is either a
// "user@host" source or the "--global" selector.
//
// The algorithm and the size bounds are always emitted: a zero value is sent
// as "inherit", which Kopia resets to its inherited default. Skipping them
// instead would make a setting impossible to clear once written, because no
// value of the configuration field could ever remove it.
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

	args = append(args,
		"--compression="+cmp.Or(policy.Algorithm, kopiaInheritPolicyValue),
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
