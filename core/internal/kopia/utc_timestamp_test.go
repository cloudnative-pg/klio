package kopia

import (
	"testing"
	"time"
)

func TestUTCTimestamp(t *testing.T) {
	now := time.Now().UTC()
	ts := UTCTimestamp(now.UnixNano())

	// Test ToTime
	if !ts.ToTime().Equal(now) {
		t.Errorf("ToTime() = %v, want %v", ts.ToTime(), now)
	}

	// Test arithmetic (Add/Sub)
	duration := 5 * time.Minute
	later := ts.Add(duration)
	if later.Sub(ts) != duration {
		t.Errorf("Sub() difference = %v, want %v", later.Sub(ts), duration)
	}

	// Test comparisons
	if !later.After(ts) {
		t.Error("After() should be true for later timestamp")
	}
	if !ts.Before(later) {
		t.Error("Before() should be true for earlier timestamp")
	}
	if !ts.Equal(UTCTimestamp(now.UnixNano())) {
		t.Error("Equal() should be true for same timestamp")
	}

	// Test formatting
	expectedFormat := now.Format(time.RFC3339)
	if got := ts.Format(time.RFC3339); got != expectedFormat {
		t.Errorf("Format() = %q, want %q", got, expectedFormat)
	}
}
