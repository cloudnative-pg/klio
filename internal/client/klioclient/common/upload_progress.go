package common

import (
	"log/slog"
)

// UploadProgress is an interface type that is used by the
// backup process to communicate back its status.
type UploadProgress interface {
	// NotifyStatus is called to notify that a certain path is being uploaded.
	NotifyStatus(source string, done uint64, estimated uint64)

	// NotifyStart is called to notify the start of the upload process
	NotifyStart(source string)

	// NotifyFinish is called to notify the end of the upload process
	NotifyFinish(source string)
}

// UploadProgressLogger is a progress implementation logging the
// status on the passed logger.
type UploadProgressLogger struct {
	log *slog.Logger
}

// NewUploadProgressLogger creates a new LoggerProcess.
func NewUploadProgressLogger(log *slog.Logger) *UploadProgressLogger {
	return &UploadProgressLogger{
		log: log,
	}
}

// NotifyStatus implements the Progress interface.
func (p *UploadProgressLogger) NotifyStatus(source string, done uint64, estimated uint64) {
	p.log.Info("Working on directory", "source", source, "done", done, "estimated", estimated)
}

// NotifyStart implements the Progress interface.
func (p *UploadProgressLogger) NotifyStart(source string) {
	p.log.Info("Start working on directory", "source", source)
}

// NotifyFinish implements the Progress interface.
func (p *UploadProgressLogger) NotifyFinish(source string) {
	p.log.Info("Stop working on directory", "source", source)
}
