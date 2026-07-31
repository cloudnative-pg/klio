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

// Package backupfailure is the single source of truth for the way a
// `klio backup run` failure is classified. It binds together, in one
// place, the failure category name (exported on the OTel
// `failure_category` attribute) and the process exit code the backup
// subprocess uses to signal that category to its parent. Keeping the
// two facets in one table prevents the exit code and the metric
// attribute from drifting apart.
package backupfailure

// Category classifies why a backup failed.
type Category struct {
	// Name is the value exported on the OTel `failure_category`
	// attribute and used throughout the documentation.
	Name string
	// ExitCode is the process exit code the `klio backup` subprocess
	// returns so its parent can recover this category, or 0 when the
	// category is derived by the parent (timeout, canceled, unknown)
	// rather than reported by the subprocess. Values follow sysexits.h.
	ExitCode int
}

// The canonical set of failure categories. The subprocess-reported
// categories carry a non-zero ExitCode; the parent-derived ones do not.
//
//nolint:gochecknoglobals
var (
	// RepositoryError marks a failure interacting with the Klio server
	// or the Kopia repository.
	RepositoryError = Category{
		Name:     "repository_error",
		ExitCode: 69, // sysexits.h EX_UNAVAILABLE (service unavailable)
	}
	// SourceError marks a failure connecting to or interacting with the
	// source PostgreSQL instance being backed up.
	SourceError = Category{
		Name:     "source_error",
		ExitCode: 68, // sysexits.h EX_NOHOST (the source host is unreachable)
	}
	// Verification marks a failure where tier-1 verification detected
	// corruption in the freshly taken backup.
	Verification = Category{
		Name:     "verification",
		ExitCode: 65, // sysexits.h EX_DATAERR (input data is incorrect)
	}
	// Timeout marks a failure where the backup exceeded its deadline.
	//
	// Unreachable in practice today: nothing sets a deadline on the
	// backup path. Kept for
	// forward-compatibility; it would fire only if CloudNativePG set a
	// Backup deadline or Klio wrapped the backup in context.WithTimeout.
	Timeout = Category{
		Name: "timeout",
	}
	// Canceled marks a failure where the backup's context was canceled
	// before a more specific category could be determined. The
	// gRPC server context is canceled when
	// the RPC is interrupted by cluster restart, hibernation, pod
	// eviction, or the instance manager disconnecting. The metric does
	// not distinguish between these causes.
	Canceled = Category{
		Name: "canceled",
	}
	// Unknown is the fallback for failures that match no other category.
	Unknown = Category{
		Name: "unknown",
	}
)

// categories lists every failure category, in the order reused for the
// user-facing documentation. It returns a fresh slice so callers cannot
// mutate the canonical set.
func categories() []Category {
	return []Category{
		RepositoryError,
		SourceError,
		Verification,
		Timeout,
		Canceled,
		Unknown,
	}
}

// ByExitCode returns the category a backup subprocess exit code maps to.
// The boolean reports whether the code matched a subprocess-reported
// category; parent-derived categories (ExitCode 0) never match.
func ByExitCode(code int) (Category, bool) {
	for _, c := range categories() {
		if c.ExitCode != 0 && c.ExitCode == code {
			return c, true
		}
	}

	return Category{}, false
}

// Names returns the name of every category, in canonical order.
func Names() []string {
	all := categories()
	names := make([]string, len(all))
	for i, c := range all {
		names[i] = c.Name
	}

	return names
}
