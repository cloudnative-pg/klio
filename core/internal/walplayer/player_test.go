package walplayer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockTaskResult is a helper for simulating TaskResults.
func mockTaskResult(name string, err string) WALUploadReport {
	return WALUploadReport{
		WALFullPath: name,
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(10 * time.Millisecond),
		Error:       err,
	}
}

func Test_runManager_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	queue := make(chan WALUploadTask)
	go runManager(ctx, dir, queue)
	count := 0
	for range queue {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 tasks, got %d", count)
	}
}

func Test_runManager_CancelsEarly(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "wal1")) //nolint: gosec
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close test file: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	queue := make(chan WALUploadTask)
	cancel() // cancel before runManager runs
	go runManager(ctx, dir, queue)

	for range queue {
		t.Error("should not send any tasks after cancel")
	}
}

func Test_runCollector_CollectsResults(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	resultsChan := make(chan WALUploadReport)

	resultsCh := make(chan []WALUploadReport)
	go func() {
		results := runCollector(ctx, resultsChan)
		resultsCh <- results
	}()

	resultsChan <- mockTaskResult("wal1", "")
	resultsChan <- mockTaskResult("wal2", "fail")
	close(resultsChan)

	results := <-resultsCh

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results[0].WALFullPath != "wal1" {
		t.Errorf("expected first result name 'wal1', got %q", results[0].WALFullPath)
	}
	if results[0].Error != "" {
		t.Errorf("expected first result to have no error, got %q", results[0].Error)
	}

	if results[1].Error != "fail" {
		t.Errorf("expected error 'fail', got %q", results[1].Error)
	}
}
