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

package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
)

// failingExitErr runs `false` and returns its *exec.ExitError-wrapping
// error. Exit code 1 maps to no category via backupfailure.ByExitCode,
// forcing the ctx.Err() fallback in classifyRunBackupError.
func failingExitErr(t *testing.T) error {
	t.Helper()
	err := exec.CommandContext(context.Background(), "false").Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)

	return err
}

func TestClassifyRunBackupErrorExitCode(t *testing.T) {
	// A real subprocess is the only portable way to get an *exec.ExitError
	// with a specific non-trivial exit code; the shell is fine for this
	// single integration check.
	cmd := exec.CommandContext(context.Background(), //nolint:gosec // hardcoded test input
		"sh", "-c", fmt.Sprintf("exit %d", backupfailure.RepositoryError.ExitCode))
	err := cmd.Run()
	require.Error(t, err)

	assert.Equal(t,
		backupfailure.RepositoryError,
		classifyRunBackupError(context.Background(), err))
}

func TestClassifyRunBackupErrorContextFallback(t *testing.T) {
	err := failingExitErr(t)

	t.Run("deadline exceeded yields timeout", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), pastInstant())
		defer cancel()
		<-ctx.Done()
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		assert.Equal(t,
			backupfailure.Timeout,
			classifyRunBackupError(ctx, err))
	})

	t.Run("canceled yields canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.Equal(t,
			backupfailure.Canceled,
			classifyRunBackupError(ctx, err))
	})

	t.Run("live context yields unknown", func(t *testing.T) {
		assert.Equal(t,
			backupfailure.Unknown,
			classifyRunBackupError(context.Background(), err))
	})
}

func TestClassifyRunBackupErrorPlainError(t *testing.T) {
	assert.Equal(t,
		backupfailure.Unknown,
		classifyRunBackupError(context.Background(), errors.New("boom")))
}

// pastInstant returns a time in the past for use with context.WithDeadline.
func pastInstant() time.Time {
	return time.Now().Add(-time.Second)
}
