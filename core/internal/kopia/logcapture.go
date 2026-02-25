package kopia

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// scanLinesOrCR is a split function for bufio.Scanner that splits on both
// '\n' and '\r'. This handles Kopia's progress output which uses '\r' for
// in-place updates. Based on bufio.ScanLines.
func scanLinesOrCR(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		// Handle \r\n as a single line ending
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			return i + 2, data[0:i], nil
		}

		return i + 1, data[0:i], nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}

// RunWithLogCapture executes a command with structured logging for stderr.
// Stderr output is logged line-by-line using structured logging.
// If captureStdout is non-nil, stdout will be written to that io.Writer for later parsing
// (without logging to reduce verbosity).
//
// Concurrency Pattern:
// This function uses StderrPipe() to capture stderr output, which requires
// a specific pattern to avoid a race condition with fast-executing subprocesses.
//
// Note: This race condition is specific to using pipes. If you directly assign io.Writer
// instances to cmd.Stdout/cmd.Stderr (as we do for stdout when captureStdout is provided),
// no special synchronization is needed because the subprocess writes directly to those
// writers without requiring goroutines.
//
// The key insight is that we wait for pipe reading to complete BEFORE calling cmd.Wait():
//
//  1. cmd.Start() - Start the subprocess
//  2. Launch goroutine to read from stderr pipe
//  3. endWg.Wait() - Wait for goroutine to finish (pipe reaches EOF)
//  4. cmd.Wait() - Reap the process
//
// Why this works:
// When stderr reaches EOF, it means the subprocess has closed the pipe.
// This only happens when the process exits (or is about to exit). Therefore, when
// endWg.Wait() returns, we know the process is done and cmd.Wait() will return
// immediately (or very quickly).
//
// Race condition prevented:
// Without this pattern, very fast commands could complete before the
// reading goroutine is scheduled by the Go runtime, potentially leading to missed
// output or timing issues. By ensuring the goroutine consumes all pipe data before we
// reap the process, we guarantee all output is captured reliably.
//
// Usage examples:
//
//	// For long-running servers that don't need stdout parsing:
//	cmd := exec.CommandContext(ctx, "kopia", "server", "start")
//	err := RunWithLogCapture(ctx, cmd, nil)
//
//	// For commands that need stdout parsing:
//	var output bytes.Buffer
//	cmd := exec.CommandContext(ctx, "kopia", "snapshot", "list")
//	err := RunWithLogCapture(ctx, cmd, &output)
//	result := parseKopiaOutput(output.String())
//
// Note: This function modifies cmd.Stdout (if captureStdout is provided) and cmd.Stderr.
// Any previous assignments to these fields will be overwritten.
func RunWithLogCapture(ctx context.Context, cmd *exec.Cmd, captureStdout io.Writer) error {
	contextLogger := log.FromContext(ctx)

	// streamLogs reads from a pipe line-by-line and logs each line with structured logging.
	// If the scanner fails (e.g., due to a line exceeding the buffer limit), we create a new
	// scanner and continue reading. This ensures the pipe keeps being drained and prevents the
	// subprocess from blocking on write.
	streamLogs := func(pipe io.Reader, stream string) {
		for {
			scanner := bufio.NewScanner(pipe)
			scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
			scanner.Split(scanLinesOrCR)

			for scanner.Scan() {
				line := scanner.Text()
				contextLogger.Info(line, "stream", stream)
			}

			if err := scanner.Err(); err != nil {
				contextLogger.Error(err, "Error reading from pipe, retrying", "stream", stream)
				continue
			}

			// scanner.Err() is nil means we reached EOF
			break
		}
	}

	// Assign stdout writer if provided (for capturing without logging)
	if captureStdout != nil {
		cmd.Stdout = captureStdout
	}

	// Create pipe for stderr (will be logged line-by-line)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Use a WaitGroup to ensure all pipe data is consumed before reaping the process.
	// This prevents a race condition where fast commands finish before goroutines start reading.
	var endWg sync.WaitGroup

	// Stream stderr - Kopia writes all logs to stderr
	endWg.Go(func() {
		streamLogs(stderrPipe, "stderr")
	})

	// Wait for the goroutine to finish reading all pipe data.
	// When this returns, the pipe has reached EOF, meaning the subprocess has
	// closed it (i.e., the process has exited or is about to exit).
	endWg.Wait()

	// Reap the process. This should return quickly since EOF means the process is done.
	cmdErr := cmd.Wait()

	if cmdErr != nil {
		return fmt.Errorf("command failed: %w", cmdErr)
	}

	return nil
}
