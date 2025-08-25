package supervisor

import (
	"context"
	"errors"
)

// EnsureProcessStarted is a helper function that starts up
// the supervised process if it was not started before.
func (s *Service) EnsureProcessStarted(ctx context.Context) error {
	err := s.StartProcess(ctx)
	if errors.Is(err, ErrProcessAlreadyStarted) {
		return nil
	}

	return err
}

// EnsureProcessStopped is a helper function that stops
// the supervised process if it was started before.
func (s *Service) EnsureProcessStopped(ctx context.Context, reason error) error {
	err := s.StopProcess(ctx, reason)
	if errors.Is(err, ErrProcessNotStarted) {
		return nil
	}

	return err
}
