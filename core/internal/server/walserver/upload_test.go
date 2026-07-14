package walserver

import (
	"errors"
	"testing"
)

// TestIsCompleted verifies that a WAL segment is considered complete only when
// the full segment has been received: a short write (interrupted by a failover)
// or a zero-length write is treated as partial.
func TestIsCompleted(t *testing.T) {
	tests := []struct {
		name        string
		writtenSize uint64
		segmentSize uint64
		want        bool
	}{
		{
			name:        "fully received segment is complete",
			writtenSize: 16,
			segmentSize: 16,
			want:        true,
		},
		{
			name:        "short write is partial",
			writtenSize: 8,
			segmentSize: 16,
			want:        false,
		},
		{
			name:        "empty write is partial",
			writtenSize: 0,
			segmentSize: 16,
			want:        false,
		},
		{
			name:        "zero-sized segment is never complete",
			writtenSize: 0,
			segmentSize: 0,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &putHandler{
				writtenSize: tt.writtenSize,
				blockMeta:   walUploadBlockMetadata{segmentSize: tt.segmentSize},
			}
			if got := h.isCompleted(); got != tt.want {
				t.Fatalf("isCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTimelineFromWALName verifies that only real WAL segments yield a timeline
// and that non-segment files (.history, .backup, .partial) return
// errNotWALSegment so they are skipped without producing a spurious parse error.
func TestTimelineFromWALName(t *testing.T) {
	tests := []struct {
		name            string
		walFileName     string
		wantTimeline    int64
		wantNotWALSegmt bool
		wantErr         bool
	}{
		{
			name:         "wal segment timeline 1",
			walFileName:  "000000010000000000000001",
			wantTimeline: 1,
		},
		{
			name:         "wal segment timeline 2",
			walFileName:  "000000020000000000000003",
			wantTimeline: 2,
		},
		{
			name:            "history file is skipped",
			walFileName:     "00000002.history",
			wantNotWALSegmt: true,
		},
		{
			name:            "backup file is skipped",
			walFileName:     "000000010000000000000002.00000028.backup",
			wantNotWALSegmt: true,
		},
		{
			name:            "partial file is skipped",
			walFileName:     "000000010000000000000003.partial",
			wantNotWALSegmt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeline, err := timelineFromWALName(tt.walFileName)

			if got := errors.Is(err, errNotWALSegment); got != tt.wantNotWALSegmt {
				t.Fatalf("errNotWALSegment: got %v, want %v (err: %v)", got, tt.wantNotWALSegmt, err)
			}
			if tt.wantNotWALSegmt {
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if timeline != tt.wantTimeline {
				t.Fatalf("unexpected timeline: got %d, want %d", timeline, tt.wantTimeline)
			}
		})
	}
}

// TestLsnStartFromWALName verifies that only real WAL segments yield a start LSN
// and that non-segment files return errNotWALSegment so they are skipped.
func TestLsnStartFromWALName(t *testing.T) {
	const segmentSize = 16 * 1024 * 1024

	tests := []struct {
		name            string
		walFileName     string
		wantStart       uint64
		wantNotWALSegmt bool
	}{
		{
			name:        "wal segment start of timeline",
			walFileName: "000000010000000000000001",
			wantStart:   16 * 1024 * 1024,
		},
		{
			name:        "wal segment second segment",
			walFileName: "000000010000000000000002",
			wantStart:   2 * 16 * 1024 * 1024,
		},
		{
			name:            "history file is skipped",
			walFileName:     "00000002.history",
			wantNotWALSegmt: true,
		},
		{
			name:            "backup file is skipped",
			walFileName:     "000000010000000000000002.00000028.backup",
			wantNotWALSegmt: true,
		},
		{
			name:            "partial file is skipped",
			walFileName:     "000000010000000000000003.partial",
			wantNotWALSegmt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startPos, err := lsnStartFromWALName(tt.walFileName, segmentSize)

			if got := errors.Is(err, errNotWALSegment); got != tt.wantNotWALSegmt {
				t.Fatalf("errNotWALSegment: got %v, want %v (err: %v)", got, tt.wantNotWALSegmt, err)
			}
			if tt.wantNotWALSegmt {
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if startPos != tt.wantStart {
				t.Fatalf("unexpected start LSN: got %d, want %d", startPos, tt.wantStart)
			}
		})
	}
}
