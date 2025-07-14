package notifier

import (
	"github.com/cloudnative-pg/machinery/pkg/log"
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

// Download is an interface type that is used by the
// restore process to communicate back the status.
type Download interface {
	// NotifyStatus is called to notify the download status.
	NotifyStatus(source string, status DownloadStats)

	// NotifyStart is called to notify the start of the download process
	NotifyStart(source string)

	// NotifyFinish is called to notify the end of the download process
	NotifyFinish(source string)
}

// downloadProgressLogger uses the logger to communicate the restore
// status.
type downloadProgressLogger struct {
	log log.Logger
}

// NewDownloadLogNotifier creates a new DownloadProcessLogger.
func NewDownloadLogNotifier(log log.Logger) Download { //nolint:nolintlint,ireturn
	return &downloadProgressLogger{
		log: log,
	}
}

// NotifyStatus implements the Notifier interface.
func (p *downloadProgressLogger) NotifyStatus(source string, stats DownloadStats) {
	p.log.Info("Working on directory", "source", source, "stats", stats)
}

// NotifyStart implements the Notifier interface.
func (p *downloadProgressLogger) NotifyStart(source string) {
	p.log.Info("Start working on directory", "source", source)
}

// NotifyFinish implements the Notifier interface.
func (p *downloadProgressLogger) NotifyFinish(source string) {
	p.log.Info("Stop working on directory", "source", source)
}
