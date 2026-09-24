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

package retention

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// applyWALRetention drops WAL files no longer required by any of
// clusterName's current backups on this tier: those with a StartWAL
// earlier than the oldest current backup's StartWAL. It re-lists backups
// itself (rather than trusting a list computed earlier in this tick) so it
// reflects reality even if a retention deletion in this same tick partially
// failed.
func (s *Sweeper) applyWALRetention(ctx context.Context, clusterName string, tc tierConfig) error {
	if tc.walRepository == nil {
		return nil
	}

	backups, err := tc.backupClient.ListBackups(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("while listing backups: %w", err)
	}
	if len(backups) == 0 {
		return nil
	}

	oldestWAL := findOldestWAL(backups)
	if oldestWAL == "" {
		return nil
	}

	if err := repository.ValidateWalFileName(oldestWAL); err != nil {
		return fmt.Errorf("computed first required WAL %q is invalid: %w", oldestWAL, err)
	}

	return tc.walRepository.SetFirstRequiredOnCluster(ctx, clusterName, oldestWAL)
}

// findOldestWAL returns the earliest StartWAL among backups, or "" if none
// have one.
func findOldestWAL(backups klioclient.BackupList) string {
	var oldest string

	for _, b := range backups {
		if b.StartWAL == "" {
			continue
		}
		if oldest == "" || b.StartWAL < oldest {
			oldest = b.StartWAL
		}
	}

	return oldest
}
