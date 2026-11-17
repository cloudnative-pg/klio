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
	"slices"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// BackupsToRelay returns the names, oldest first, of the backups of a
// cluster that a relay must copy from tier1 to tier2. tier1 and tier2 are
// the backups complete on each tier, and partial names the ones that tier2
// holds only in part.
//
// A backup missing from tier2 is relayed only when tier2Policy keeps it
// there: the tier2 sweep would delete any other on arrival, so a relay never
// copies back a backup that the sweep removed. A backup in partial is always
// relayed, to complete it: the sweep never deletes an incomplete backup. A
// nil tier2Policy keeps every backup.
func BackupsToRelay(
	tier2Policy *grpc.TierRetentionPolicy,
	tier1, tier2 klioclient.BackupList,
	partial []string,
) ([]string, error) {
	evaluator, err := policyFromProto(tier2Policy)
	if err != nil {
		return nil, err
	}

	onTier2 := backupNameSet(tier2)

	missing := make(klioclient.BackupList, 0, len(tier1))
	for _, b := range tier1 {
		if _, ok := onTier2[b.Name]; !ok {
			missing = append(missing, b)
		}
	}
	missing.SortByAscendingTime()

	// What tier2 holds once every missing backup is there and the sweep
	// has run. Evaluate sorts and slices in place: give it its own slice.
	kept := backupNameSet(evaluator.Evaluate(slices.Concat(tier2, missing)))

	names := make([]string, 0, len(missing))
	for _, b := range missing {
		if _, ok := kept[b.Name]; ok || slices.Contains(partial, b.Name) {
			names = append(names, b.Name)
		}
	}

	return names, nil
}
