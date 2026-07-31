/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package supervisor

import (
	"context"
	"sync"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// Service is the supervisor implementation for a single process.
type Service struct {
	// Definition is the service definition
	Definition Definition

	// The supervisor wakes up at least every tickInterval to monitor
	// the current status of the process.
	tickInterval time.Duration

	// StopTime is the time the latest process was terminated
	StopTime time.Time

	// StopRequestTime is the time the latest process stop was requested
	StopRequestTime time.Time

	// StartTime is the time the process was started
	StartTime time.Time

	// LastAutomaticRestart is the time the process was automatically restarted
	LastAutomaticRestart time.Time

	// Error is the latest detected process error
	Error error

	m                 sync.Mutex
	processCancelFunc context.CancelCauseFunc
	startRequests     chan any
}

// NewService creates a service for a process given its
// definition.
func NewService(definition *Definition) *Service {
	return &Service{
		Definition:    *definition,
		tickInterval:  10 * time.Second,
		startRequests: make(chan any, 1),
	}
}

// StartProcess starts the monitored process.
func (s *Service) StartProcess(ctx context.Context) error {
	if s.processCancelFunc != nil {
		return ErrProcessAlreadyStarted
	}

	logger := log.FromContext(ctx)
	logger.Info("Requested process start", "definition", s.Definition)

	// non-blocking enqueue if already pending
	select {
	case s.startRequests <- nil:
	default:
	}

	return nil
}

// StopProcess stops the monitored process, and avoids any successive
// automatic restarts.
func (s *Service) StopProcess(ctx context.Context, reason error) error {
	s.m.Lock()
	defer func() {
		s.m.Unlock()
	}()

	if s.processCancelFunc == nil {
		return ErrProcessNotStarted
	}

	logger := log.FromContext(ctx)
	logger.Info("Requested process stop", "definition", s.Definition, "reason", reason)

	s.StopRequestTime = time.Now()
	s.processCancelFunc(reason)

	return nil
}

// Start starts the supervisor for the service, without
// automatically starting the subprocess.
func (s *Service) Start(ctx context.Context) error {
	logger := log.FromContext(ctx)

	tick := time.NewTicker(s.tickInterval)
	defer func() {
		// Yes, with newer Go versions this is not really
		// needed, but it is better to be safe than sorry.
		tick.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-s.startRequests:
			// We process a start request
			s.internalStartProcess(ctx)

		case <-tick.C:
			// We may need to trigger an automatic restart
			if err := s.triggerRestartWhenNeeded(ctx); err != nil {
				logger.Error(
					err,
					"Error while triggering subprocess restart, we'll retry later",
					"definition", s.Definition,
				)
			}
		}
	}
}

// IsProcessRunning returns true if the subprocess is running.
func (s *Service) IsProcessRunning() bool {
	return s.processCancelFunc != nil
}

func (s *Service) internalStartProcess(ctx context.Context) {
	s.m.Lock()
	logger := log.FromContext(ctx)
	logger.Info("Processing process start request", "definition", s.Definition)

	processContext, processCancelFunc := context.WithCancelCause(ctx)
	cmd := s.Definition.NewCmd(processContext)

	s.StartTime = time.Now()
	s.StopTime = time.Time{}
	s.StopRequestTime = time.Time{}
	s.processCancelFunc = processCancelFunc
	s.Error = nil
	s.m.Unlock()

	go func() {
		processError := cmd.Run()
		logger.Info("Process finished", "processError", processError)

		s.m.Lock()
		s.StopTime = time.Now()
		s.Error = processError
		s.processCancelFunc = nil
		s.m.Unlock()
	}()
}

func (s *Service) triggerRestartWhenNeeded(ctx context.Context) error {
	logger := log.FromContext(ctx)

	if !s.Definition.AutoRestart { // feature gate
		return nil
	}

	// Skip restart if the process was manually stopped (we recorded a stop request time)
	if !s.StopRequestTime.IsZero() {
		logger.Debug("Avoid triggering a restart for a process that was manually stopped")
		return nil
	}

	// There is no need to restart a process that was never started
	if s.StartTime.IsZero() {
		return nil
	}

	// There is no need to restart a process that is running
	if s.StopTime.IsZero() {
		return nil
	}

	// There is no need to restart a process if the waiting time is too low.
	if time.Since(s.LastAutomaticRestart) < s.Definition.RestartWaitPeriod {
		logger.Debug("Avoid triggering a restart for a process that was restarted too recently")
		return nil
	}

	s.m.Lock()
	s.LastAutomaticRestart = time.Now()
	s.m.Unlock()
	logger.Info("Triggering automatic restart", "definition", s.Definition)

	return s.StartProcess(ctx)
}
