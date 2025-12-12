package kopiaserver

import (
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
)

func TestSnapshotStats_Increment_FirstSnapshot_ShouldInitializeAndUpdate(t *testing.T) {
	s := snapshotStats{}
	ds := kopia.DirectorySummary{
		TotalFileSize:  123,
		TotalFileCount: 5,
		TotalDirCount:  2,
	}

	s.update(42, &ds)

	if s.snapshotCount != 1 {
		t.Fatalf("expected snapshotCount=1, got %d (deferred update on value receiver is lost)", s.snapshotCount)
	}
	if s.oldestSnapshotAge != 42 || s.latestSnapshotAge != 42 {
		t.Fatalf("expected ages oldest=42 and latest=42, got oldest=%v latest=%v", s.oldestSnapshotAge, s.latestSnapshotAge)
	}
	if s.snapshotSize != 123 || s.latestSnapshotFilesCount != 5 || s.latestSnapshotDirCount != 2 {
		t.Fatalf("expected latest snapshot details from ds, got size=%d files=%d dirs=%d",
			s.snapshotSize, s.latestSnapshotFilesCount, s.latestSnapshotDirCount)
	}
}
