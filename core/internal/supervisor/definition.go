package supervisor

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// Definition contains the information needed to start a subprocess.
type Definition struct {
	// Exec is the name of the executable
	Exec string

	// Args are the command line arguments
	Args []string

	// AutoRestart is true if the process should be automatically
	// restarted on failure
	AutoRestart bool

	// RestartWaitPeriod is waited between automatic process restarts
	RestartWaitPeriod time.Duration
}

// NewCmd creates a new *Cmd for the given process definition.
func (d *Definition) NewCmd(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, d.Exec, d.Args...) //nolint:gosec
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	return cmd
}
