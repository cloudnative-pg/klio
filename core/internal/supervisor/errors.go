package supervisor

import "errors"

// ErrProcessNotStarted is raised when the requested action is not available
// because the process was not started.
var ErrProcessNotStarted = errors.New("process not started")

// ErrProcessAlreadyStarted is raised when the requested action is not available
// because the process was already started.
var ErrProcessAlreadyStarted = errors.New("process already started")
