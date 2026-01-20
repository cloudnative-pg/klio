package kopia

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWithLogCapture_Success(t *testing.T) {
	ctx := context.Background()

	// Test a simple command that succeeds
	cmd := exec.CommandContext(ctx, "echo", "hello world")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.NoError(t, err)
}

func TestRunWithLogCapture_CommandFailure(t *testing.T) {
	ctx := context.Background()

	// Test a command that fails
	cmd := exec.CommandContext(ctx, "sh", "-c", "exit 1")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command failed")
}

func TestRunWithLogCapture_WithStdoutCapture(t *testing.T) {
	ctx := context.Background()

	// Test stdout capture
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "echo", "test output")

	err := RunWithLogCapture(ctx, cmd, &stdout)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "test output")
}

func TestRunWithLogCapture_StderrCapture(t *testing.T) {
	ctx := context.Background()

	// Test a command that writes to stderr
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'error message' >&2")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.NoError(t, err)
}

func TestRunWithLogCapture_BothStreams(t *testing.T) {
	ctx := context.Background()

	// Test a command that writes to both stdout and stderr
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'stdout line'; echo 'stderr line' >&2")

	err := RunWithLogCapture(ctx, cmd, &stdout)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "stdout line")
}

func TestRunWithLogCapture_MultilineOutput(t *testing.T) {
	ctx := context.Background()

	// Test a command with multiline output
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'line 1'; echo 'line 2'; echo 'line 3'")

	err := RunWithLogCapture(ctx, cmd, &stdout)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "line 1")
	assert.Contains(t, output, "line 2")
	assert.Contains(t, output, "line 3")
}

func TestRunWithLogCapture_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test with a canceled context
	cmd := exec.CommandContext(ctx, "sleep", "10")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.Error(t, err)
}

func TestRunWithLogCapture_InvalidCommand(t *testing.T) {
	ctx := context.Background()

	// Test with a command that doesn't exist
	cmd := exec.CommandContext(ctx, "nonexistent-command-xyz")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "failed to start")
}
