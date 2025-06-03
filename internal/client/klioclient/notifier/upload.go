package notifier

import (
	"log/slog"
)

// Upload is an interface type that is used by the
// backup process to communicate back its status.
type Upload interface {
	// NotifyStatus is called to notify that a certain path is being uploaded.
	NotifyStatus(source string, done uint64, estimated uint64)

	// NotifyStart is called to notify the start of the upload process
	NotifyStart(source string)

	// NotifyFinish is called to notify the end of the upload process
	NotifyFinish(source string)
}

// uploadProgressLogger is a progress implementation logging the
// status on the passed logger.
type uploadProgressLogger struct {
	log *slog.Logger
}

// NewUploadLogNotifier creates a new LoggerProcess.
func NewUploadLogNotifier(log *slog.Logger) Upload { //nolint:ireturn
	return &uploadProgressLogger{
		log: log,
	}
}

// NotifyStatus implements the Notifier interface.
func (p *uploadProgressLogger) NotifyStatus(source string, done uint64, estimated uint64) {
	p.log.Info("Working on directory", "source", source, "done", done, "estimated", estimated)
}

// NotifyStart implements the Notifier interface.
func (p *uploadProgressLogger) NotifyStart(source string) {
	p.log.Info("Start working on directory", "source", source)
}

// NotifyFinish implements the Notifier interface.
func (p *uploadProgressLogger) NotifyFinish(source string) {
	p.log.Info("Stop working on directory", "source", source)
}
