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

package kopia

import (
	"context"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// SetRetentionPolicy sets the retention policy for backups of this cluster.
func (s *Connection) SetRetentionPolicy(ctx context.Context, t kopia.Target, p kopia.RetentionPolicy) error {
	return s.kopia.SetKopiaPolicy(ctx, t, &p)
}

// GetRetentionPolicy gets the currently applied retention policy for this cluster.
func (s *Connection) GetRetentionPolicy(ctx context.Context, t kopia.Target) (*kopia.RetentionPolicy, error) {
	policy, err := s.kopia.GetCurrentKopiaPolicy(ctx, t)
	if err != nil {
		return nil, err
	}

	return &policy.RetentionPolicy, nil
}

// ApplyRetentionPolicy applies the retention policy for this cluster, deleting any
// snapshots that are no longer needed.
func (s *Connection) ApplyRetentionPolicy(ctx context.Context, t kopia.Target) error {
	return s.kopia.ApplyKopiaPolicy(ctx, t)
}
