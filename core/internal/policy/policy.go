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

// Package policy implements the retention strategies the retention sweeper
// (internal/retention) evaluates a cluster's backups against.
package policy

import (
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// Policy decides which of a cluster's backups to keep.
type Policy interface {
	// Evaluate returns the subset of backupList that should be kept; the
	// rest are out of retention.
	Evaluate(backupList klioclient.BackupList) klioclient.BackupList
}

// LatestPolicy keeps the Count most recent backups. Count == 0 disables
// retention: every backup is kept, the same as having no policy at all.
type LatestPolicy struct {
	Count int
}

// Evaluate implements Policy.
func (p *LatestPolicy) Evaluate(backupList klioclient.BackupList) klioclient.BackupList {
	if p.Count == 0 {
		return backupList
	}

	backupList.SortByAscendingTime()
	// Keep only the latest 'Count' backups
	if len(backupList) > p.Count {
		backupList = backupList[len(backupList)-p.Count:]
	}

	return backupList
}
