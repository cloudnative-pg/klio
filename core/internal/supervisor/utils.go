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
