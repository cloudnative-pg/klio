package cli

import "errors"

var (
	// ErrKopiaClientSectionIsRequired is raised when the kopia client configuration is missing.
	ErrKopiaClientSectionIsRequired = errors.New("'client.kopia' configuration section is required")

	// ErrSourceSectionIsRequired is raised when the WAL pusher is started without a
	// source specification.
	ErrSourceSectionIsRequired = errors.New("'source' configuration section is required")

	// ErrClientSectionIsRequired is raised when the WAL pusher is started without a
	// client specification.
	ErrClientSectionIsRequired = errors.New("'client' configuration section is required")

	// ErrKlioClientSectionIsRequired is raised when the Klio client configuration is missing.
	ErrKlioClientSectionIsRequired = errors.New("'client.wal' configuration section is required")
)

// ExitCoder is implemented by errors that carry a subprocess exit
// code. Klio cli subcommands extract the code via errors.As and pass
// it to os.Exit so the parent can classify the failure.
type ExitCoder interface {
	error
	ExitCode() int
}

// CodedError tags an error with the exit code the subprocess should
// return so the parent CNPG sidecar can classify the failure.
type CodedError struct {
	err  error
	code int
}

// NewCodedError wraps err so the subprocess exits with the given
// classification code. Returns a nil error when err is nil so the
// result can be returned directly from a function whose signature
// is `error` without triggering the typed-nil-interface footgun.
func NewCodedError(err error, code int) error {
	if err == nil {
		return nil
	}

	return &CodedError{err: err, code: code}
}

// Error implements the error interface.
func (e *CodedError) Error() string {
	return e.err.Error()
}

// Unwrap exposes the wrapped error so errors.Is/As walk through.
func (e *CodedError) Unwrap() error {
	return e.err
}

// ExitCode implements ExitCoder.
func (e *CodedError) ExitCode() int {
	return e.code
}
