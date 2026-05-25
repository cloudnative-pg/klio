package kopia

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWithLogCaptureSuccess(t *testing.T) {
	ctx := context.Background()

	cmd := exec.CommandContext(ctx, "echo", "hello world")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.NoError(t, err)
}

func TestRunWithLogCaptureCommandFailure(t *testing.T) {
	ctx := context.Background()

	// Test a command that fails
	cmd := exec.CommandContext(ctx, "sh", "-c", "exit 1")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command failed")
}

func TestRunWithLogCaptureWithStdoutCapture(t *testing.T) {
	ctx := context.Background()

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "echo", "test output")

	err := RunWithLogCapture(ctx, cmd, &stdout)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "test output")
}

func TestRunWithLogCaptureStderrCapture(t *testing.T) {
	ctx := context.Background()

	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'error message' >&2")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.NoError(t, err)
}

func TestRunWithLogCaptureBothStreams(t *testing.T) {
	ctx := context.Background()

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'stdout line'; echo 'stderr line' >&2")

	err := RunWithLogCapture(ctx, cmd, &stdout)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "stdout line")
}

func TestRunWithLogCaptureMultilineOutput(t *testing.T) {
	ctx := context.Background()

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'line 1'; echo 'line 2'; echo 'line 3'")

	err := RunWithLogCapture(ctx, cmd, &stdout)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "line 1")
	assert.Contains(t, output, "line 2")
	assert.Contains(t, output, "line 3")
}

func TestRunWithLogCaptureCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test with a canceled context
	cmd := exec.CommandContext(ctx, "sleep", "10")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.Error(t, err)
}

func TestRunWithLogCaptureInvalidCommand(t *testing.T) {
	ctx := context.Background()

	// Test with a command that doesn't exist
	cmd := exec.CommandContext(ctx, "nonexistent-command-xyz")

	err := RunWithLogCapture(ctx, cmd, nil)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "failed to start")
}

func TestIsDiskFullMessage(t *testing.T) {
	testCases := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "typical ENOSPC from Kopia session",
			line: `error handling session request: write /data/base/kopia.repository/p123: no space left on device`,
			want: true,
		},
		{
			name: "ENOSPC with mixed case",
			line: `No Space Left On Device`,
			want: true,
		},
		{
			name: "normal session start message",
			line: `starting session for user "klio@my-cluster" from 10.0.0.5:41234`,
			want: false,
		},
		{
			name: "normal session ended message",
			line: `session ended for user "klio@my-cluster" from 10.0.0.5:41234`,
			want: false,
		},
		{
			name: "empty line",
			line: "",
			want: false,
		},
		{
			name: "generic error without disk full",
			line: "error handling session request: connection reset by peer",
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isDiskFullMessage(tc.line))
		})
	}
}

func TestScanLinesOrCR(t *testing.T) {
	testCases := []struct {
		name        string
		input       []byte
		atEOF       bool
		wantAdvance int
		wantToken   string
		wantMore    bool // true if we expect (0, nil, nil) - requesting more data
	}{
		{
			name:        "line ending with LF",
			input:       []byte("hello\nworld"),
			atEOF:       false,
			wantAdvance: 6,
			wantToken:   "hello",
		},
		{
			name:        "line ending with CR",
			input:       []byte("hello\rworld"),
			atEOF:       false,
			wantAdvance: 6,
			wantToken:   "hello",
		},
		{
			name:        "line ending with CRLF",
			input:       []byte("hello\r\nworld"),
			atEOF:       false,
			wantAdvance: 7,
			wantToken:   "hello",
		},
		{
			name:        "empty input at EOF",
			input:       []byte{},
			atEOF:       true,
			wantAdvance: 0,
			wantToken:   "",
		},
		{
			name:        "empty input not at EOF",
			input:       []byte{},
			atEOF:       false,
			wantAdvance: 0,
			wantToken:   "",
			wantMore:    true,
		},
		{
			name:        "no line ending at EOF returns final line",
			input:       []byte("final"),
			atEOF:       true,
			wantAdvance: 5,
			wantToken:   "final",
		},
		{
			name:        "no line ending not at EOF requests more data",
			input:       []byte("partial"),
			atEOF:       false,
			wantAdvance: 0,
			wantToken:   "",
			wantMore:    true,
		},
		{
			name:        "only LF",
			input:       []byte("\n"),
			atEOF:       false,
			wantAdvance: 1,
			wantToken:   "",
		},
		{
			name:        "only CR",
			input:       []byte("\r"),
			atEOF:       false,
			wantAdvance: 1,
			wantToken:   "",
		},
		{
			name:        "only CRLF",
			input:       []byte("\r\n"),
			atEOF:       false,
			wantAdvance: 2,
			wantToken:   "",
		},
		{
			name:        "CR at end of buffer not at EOF",
			input:       []byte("hello\r"),
			atEOF:       false,
			wantAdvance: 6,
			wantToken:   "hello",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			advance, token, err := scanLinesOrCR(tc.input, tc.atEOF)
			require.NoError(t, err)

			if tc.wantMore {
				assert.Equal(t, 0, advance, "expected request for more data")
				assert.Nil(t, token, "expected nil token when requesting more data")
			} else {
				assert.Equal(t, tc.wantAdvance, advance)
				assert.Equal(t, tc.wantToken, string(token))
			}
		})
	}
}

func TestScanLinesOrCRFullScan(t *testing.T) {
	// Test scanning a complete input with mixed line endings using bufio.Scanner.
	testCases := []struct {
		name      string
		input     string
		wantLines []string
	}{
		{
			name:      "mixed LF and CR",
			input:     "line1\nline2\rline3\n",
			wantLines: []string{"line1", "line2", "line3"},
		},
		{
			name:      "all CR (progress-style output)",
			input:     "progress 10%\rprogress 50%\rprogress 100%\r",
			wantLines: []string{"progress 10%", "progress 50%", "progress 100%"},
		},
		{
			name:      "CRLF line endings",
			input:     "line1\r\nline2\r\nline3\r\n",
			wantLines: []string{"line1", "line2", "line3"},
		},
		{
			name:      "mixed endings with final line without terminator",
			input:     "line1\nline2\rline3",
			wantLines: []string{"line1", "line2", "line3"},
		},
		{
			name:      "empty lines",
			input:     "line1\n\nline2\r\rline3",
			wantLines: []string{"line1", "", "line2", "", "line3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var lines []string
			scanner := bufio.NewScanner(strings.NewReader(tc.input))
			scanner.Split(scanLinesOrCR)

			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			require.NoError(t, scanner.Err())
			assert.Equal(t, tc.wantLines, lines)
		})
	}
}
