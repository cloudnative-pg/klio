package common

import (
	"log/slog"
)

// Progress is an interface type that is used by the
// backup process to communicate back its status.
type Progress interface {
	// NotifyStatus is called to notify that a certain path is being uploaded.
	NotifyStatus(source string, uploaded uint64, estimated uint64)

	// NotifyStart is called to notify the start of the upload process
	NotifyStart(source string)

	// NotifyFinish is called to notify the end of the upload process
	NotifyFinish(source string)
}

// LoggerProgress is a progress implementation logging the
// status on the passed logger.
type LoggerProgress struct {
	log *slog.Logger
}

// NewLoggerProgress creates a new LoggerProcess.
func NewLoggerProgress(log *slog.Logger) *LoggerProgress {
	return &LoggerProgress{
		log: log,
	}
}

// NotifyStatus implements the Progress interface.
func (p *LoggerProgress) NotifyStatus(source string, uploaded uint64, estimated uint64) {
	p.log.Info("Uploading", "source", source, "uploaded", uploaded, "estimated", estimated)
}

// NotifyStart implements the Progress interface.
func (p *LoggerProgress) NotifyStart(source string) {
	p.log.Info("Start uploading", "source", source)
}

// NotifyFinish implements the Progress interface.
func (p *LoggerProgress) NotifyFinish(source string) {
	p.log.Info("Stop uploading", "source", source)
}
