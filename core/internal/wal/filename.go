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

package wal

import (
	"strings"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
)

// PartialSuffix is the suffix carried by a WAL segment that was not fully
// received, such as one left behind by a failover.
const PartialSuffix = ".partial"

// IsPartialWALFile reports whether name is the .partial variant of a complete
// WAL segment, such as one left behind by a failover.
func IsPartialWALFile(name string) bool {
	bareName, isPartial := strings.CutSuffix(name, PartialSuffix)
	return isPartial && postgres.IsWALFile(bareName)
}

// IsWALSegmentOrPartial reports whether name is a complete WAL segment or its
// .partial variant.
func IsWALSegmentOrPartial(name string) bool {
	return postgres.IsWALFile(name) || IsPartialWALFile(name)
}

// TrimPartialSuffix returns name without the .partial suffix.
func TrimPartialSuffix(name string) string {
	return strings.TrimSuffix(name, PartialSuffix)
}

// WithPartialSuffix returns name with the .partial suffix, if it does not already have it.
func WithPartialSuffix(name string) string {
	if !strings.HasSuffix(name, PartialSuffix) {
		return name + PartialSuffix
	}

	return name
}
