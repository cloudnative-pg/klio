package kopia

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// PinSnapshotOpts contains options for pinning or unpinning snapshots.
type PinSnapshotOpts struct {
	// The set of snapshots or root objects IDs to be pinned or unpinned.
	IDs []string

	// AddPins is the list of pins to be added.
	AddPins []string

	// RemovePins is the list of pins to be removed.
	RemovePins []string
}

// PinSnapshots pins and unpins a set of snapshots.
func (s *Client) PinSnapshots(ctx context.Context, opts PinSnapshotOpts) error {
	if len(opts.IDs) == 0 {
		return nil
	}

	contextLogger := log.FromContext(ctx)

	args := make([]string, 0, 4+len(opts.AddPins)+len(opts.RemovePins)+len(opts.IDs))
	args = append(args,
		"snapshot",
		"pin",
		"--config-file="+s.ConfigFile,
		"--disable-file-logging",
	)

	for _, pin := range opts.AddPins {
		args = append(args, "--add="+pin)
	}

	for _, pin := range opts.RemovePins {
		args = append(args, "--remove="+pin)
	}

	args = append(args, opts.IDs...)

	contextLogger.Info("Pinning/unpinning Kopia snapshot",
		"args", args, "addPins", opts.AddPins, "removePins", opts.RemovePins)

	pinSnapshotCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	pinSnapshotCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, pinSnapshotCmd, nil); err != nil {
		return fmt.Errorf("while pinning/unpinning Kopia snapshot: %w", err)
	}

	return nil
}
