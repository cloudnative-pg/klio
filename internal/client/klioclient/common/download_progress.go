package common

import (
	"log/slog"
)

// DownloadStats represents restore statistics.
// This structure is modeled from the relative Kopia
// API and kept here to ensure compatibility with multiple backends.
type DownloadStats struct {
	RestoredTotalFileSize int64 `json:"restoredTotalFileSize"`
	EnqueuedTotalFileSize int64 `json:"enqueuedTotalFileSize"`
	SkippedTotalFileSize  int64 `json:"skippedTotalFileSize"`

	RestoredFileCount    int32 `json:"restoredFileCount"`
	RestoredDirCount     int32 `json:"restoredDirCount"`
	RestoredSymlinkCount int32 `json:"restoredSymlinkCount"`
	EnqueuedFileCount    int32 `json:"enqueuedFileCount"`
	EnqueuedDirCount     int32 `json:"enqueuedDirCount"`
	EnqueuedSymlinkCount int32 `json:"enqueuedSymlinkCount"`
	SkippedCount         int32 `json:"skippedCount"`
	IgnoredErrorCount    int32 `json:"ignoredErrorCount"`
}

// DownloadProgress is an interface type that is used by the
// restore process to communicate back the status.
type DownloadProgress interface {
	// NotifyStatus is called to notify that a certain path is being uploaded.
	NotifyStatus(source string, stats DownloadStats)

	// NotifyStart is called to notify the start of the upload process
	NotifyStart(source string)

	// NotifyFinish is called to notify the end of the upload process
	NotifyFinish(source string)
}

// DownloadProgressLogger uses the logger to communicate the restore
// status
type DownloadProgressLogger struct {
	log *slog.Logger
}

// NewDownloadProgressLogger creates a new DownloadProcessLogger
func NewDownloadProgressLogger(log *slog.Logger) *DownloadProgressLogger {
	return &DownloadProgressLogger{
		log: log,
	}
}

// NotifyStatus implements the Progress interface.
func (p *DownloadProgressLogger) NotifyStatus(source string, stats DownloadStats) {
	p.log.Info("Working on directory", "source", source, "stats", stats)
}

// NotifyStart implements the Progress interface.
func (p *DownloadProgressLogger) NotifyStart(source string) {
	p.log.Info("Start working on directory", "source", source)
}

// NotifyFinish implements the Progress interface.
func (p *DownloadProgressLogger) NotifyFinish(source string) {
	p.log.Info("Stop working on directory", "source", source)
}
