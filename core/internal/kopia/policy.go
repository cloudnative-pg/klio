package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	showPolicyCmd.Stdout = &buffer
	showPolicyCmd.Stderr = os.Stderr
	showPolicyCmd.Env = s.envPassword()

	if err := showPolicyCmd.Run(); err != nil {
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
		"--json-log-console",
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
	setPolicyCmd.Stdout = os.Stdout
	setPolicyCmd.Stderr = os.Stderr
	setPolicyCmd.Env = s.envPassword()

	if err := setPolicyCmd.Run(); err != nil {
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
		"--json-log-console",
		t.String(),
	}

	contextLogger.Info("Applying Kopia policy", "args", args, "target", t)

	snapshotExpireCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	snapshotExpireCmd.Stdout = os.Stdout
	snapshotExpireCmd.Stderr = os.Stderr
	snapshotExpireCmd.Env = s.envPassword()

	if err := snapshotExpireCmd.Run(); err != nil {
		return fmt.Errorf("error while applying Kopia policy: %w", err)
	}

	return nil
}
