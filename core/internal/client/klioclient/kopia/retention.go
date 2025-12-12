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

// SetRetentionPolicy sets the retention policy for backups of this cluster.
func (s *Connection) SetRetentionPolicy(ctx context.Context, t Target, p RetentionPolicy) error {
	return s.internalSetKopiaPolicy(ctx, t, &p)
}

// GetRetentionPolicy gets the currently applied retention policy for this cluster.
func (s *Connection) GetRetentionPolicy(ctx context.Context, t Target) (*RetentionPolicy, error) {
	currentPolicy, err := s.internalGetCurrentKopiaPolicy(ctx, t)
	if err != nil {
		return nil, err
	}

	if currentPolicy == nil {
		return nil, nil
	}

	return &currentPolicy.RetentionPolicy, nil
}

// ApplyRetentionPolicy applies the retention policy for this cluster, deleting any
// snapshots that are no longer needed.
func (s *Connection) ApplyRetentionPolicy(ctx context.Context, t Target) error {
	return s.internalApplyKopiaPolicy(ctx, t)
}

func (s *Connection) internalGetCurrentKopiaPolicy(ctx context.Context, t Target) (*Policy, error) {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"policy",
		"show",
		t.String(),
		"--config-file=" + s.configFile,
		"--disable-file-logging",
		"--json",
		"--password=mtls",
	}

	contextLogger.Info("Getting Kopia policy", "args", args, "target", t)

	var buffer bytes.Buffer

	showPolicyCmd := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	showPolicyCmd.Stdout = &buffer
	showPolicyCmd.Stderr = os.Stderr

	if err := showPolicyCmd.Run(); err != nil {
		return nil, fmt.Errorf("error while getting Kopia policy: %w", err)
	}

	var result Policy
	if err := json.NewDecoder(&buffer).Decode(&result); err != nil {
		return nil, fmt.Errorf("cannot decode JSON backup metadata: %w", err)
	}

	return &result, nil
}

func (s *Connection) internalSetKopiaPolicy(ctx context.Context, t Target, policy *RetentionPolicy) error {
	policyToArgument := func(value *int) string {
		if value == nil {
			return "none"
		}

		return strconv.Itoa(*value)
	}

	contextLogger := log.FromContext(ctx)

	args := []string{
		"policy",
		"set",
		"--config-file=" + s.configFile,
		"--disable-file-logging",
		"--json-log-console",
		"--password=mtls",
		"--keep-annual=" + policyToArgument(policy.KeepAnnual),
		"--keep-daily=" + policyToArgument(policy.KeepDaily),
		"--keep-hourly=" + policyToArgument(policy.KeepHourly),
		"--keep-latest=" + policyToArgument(policy.KeepLatest),
		"--keep-monthly=" + policyToArgument(policy.KeepMonthly),
		"--keep-weekly=" + policyToArgument(policy.KeepWeekly),
		t.String(),
	}

	contextLogger.Info("Setting Kopia policy", "args", args, "target", t)

	setPolicyCmd := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	setPolicyCmd.Stdout = os.Stdout
	setPolicyCmd.Stderr = os.Stderr

	if err := setPolicyCmd.Run(); err != nil {
		return fmt.Errorf("error while setting Kopia policy: %w", err)
	}

	return nil
}

func (s *Connection) internalApplyKopiaPolicy(ctx context.Context, t Target) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"expire",
		"--config-file=" + s.configFile,
		"--disable-file-logging",
		"--json-log-console",
		"--password=mtls",
		t.String(),
	}

	contextLogger.Info("Applying Kopia policy", "args", args, "target", t)

	snapshotExpireCmd := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	snapshotExpireCmd.Stdout = os.Stdout
	snapshotExpireCmd.Stderr = os.Stderr

	if err := snapshotExpireCmd.Run(); err != nil {
		return fmt.Errorf("error while applying Kopia policy: %w", err)
	}

	return nil
}
