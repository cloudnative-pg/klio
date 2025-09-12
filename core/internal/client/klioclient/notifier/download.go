package notifier

import (
	"github.com/cloudnative-pg/machinery/pkg/log"
)

// DownloadProgressLogger uses the logger to communicate the restore
// status.
type DownloadProgressLogger struct {
	log log.Logger
}

// NewDownloadLogNotifier creates a new DownloadProcessLogger.
func NewDownloadLogNotifier(log log.Logger) *DownloadProgressLogger {
	return &DownloadProgressLogger{
		log: log,
	}
}

// NotifyStatus implements the Notifier interface.
func (p *DownloadProgressLogger) NotifyStatus(source string, stats DownloadStats) {
	p.log.Info("Working on directory", "source", source, "stats", stats)
}

// NotifyStart implements the Notifier interface.
func (p *DownloadProgressLogger) NotifyStart(source string) {
	p.log.Info("Start working on directory", "source", source)
}

// NotifyFinish implements the Notifier interface.
func (p *DownloadProgressLogger) NotifyFinish(source string) {
	p.log.Info("Stop working on directory", "source", source)
}
