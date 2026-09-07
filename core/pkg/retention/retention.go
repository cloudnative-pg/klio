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

// Package retention evaluates Klio-managed retention policies against a backup
// catalog. It is intentionally free of any Kopia or I/O dependency so that the
// decision of which backups have expired can be tested in isolation and reused
// by other consumers of the same catalog format.
package retention

import "sort"

// Policy describes which backups Klio should keep. Its zero value means "no
// retention configured", which keeps every backup; a configured policy always
// carries a positive count, as values below 1 are rejected.
type Policy struct {
	// Latest keeps only the N most recent backups and expires the rest. A value
	// below 1 means no retention is configured, keeping everything.
	Latest int `json:"latest,omitempty" mapstructure:"latest"`
}

// Backup is the minimal view of a backup that retention evaluation needs. The
// catalog passed to Evaluate is a slice of these, decoupled from how the
// backups are actually stored.
type Backup struct {
	// Name uniquely identifies the backup within a cluster.
	Name string

	// StartedAt is the backup start time, in Unix seconds. It is the
	// chronological key used to order the catalog.
	StartedAt int64

	// StoppedAt is the backup completion time, in Unix seconds.
	StoppedAt int64
}

// Evaluate returns the backups in catalog that fall outside policy and should
// therefore be deleted. The input slice is not modified, and a backup is never
// returned more than once.
//
// For the "latest" criterion the N most recent backups (ordered by StartedAt,
// most recent first) are kept and every older backup is expired. A policy that
// keeps everything (see Policy.IsZero) yields no expired backups.
func Evaluate(catalog []Backup, policy Policy) []Backup {
	// A count below 1 means no retention is configured (see Policy): keep
	// everything. This also guards the slice bound below.
	keep := policy.Latest
	if keep < 1 {
		return nil
	}

	if len(catalog) <= keep {
		return nil
	}

	// Order a copy from most to least recent so the survivors are the newest
	// backups. StartedAt is the primary key; the name breaks ties so the
	// outcome is deterministic when two backups share a start time.
	ordered := make([]Backup, len(catalog))
	copy(ordered, catalog)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].StartedAt != ordered[j].StartedAt {
			return ordered[i].StartedAt > ordered[j].StartedAt
		}

		return ordered[i].Name > ordered[j].Name
	})

	expired := make([]Backup, len(ordered)-keep)
	copy(expired, ordered[keep:])

	return expired
}
