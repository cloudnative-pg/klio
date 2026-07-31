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
