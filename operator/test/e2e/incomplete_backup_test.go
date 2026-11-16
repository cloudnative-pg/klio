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

package e2e

import (
	klioFeatures "github.com/cloudnative-pg/klio/operator/test/klio/features"
)

// IncompleteBackup returns an IncompleteBackupFeature verifying that a
// backup missing one of its snapshots is not listed, is reported with the
// right status, survives the retention sweep, and is either deleted by
// hand or rebuilt by a relay. It reuses the retention scenario with tier1
// keeping its latest backup and tier2 keeping more backups than the test
// takes, so tier2 retention never deletes anything.
func IncompleteBackup(namespace string) *klioFeatures.IncompleteBackupFeature {
	cfg := newRetentionFeatureConfig("IncompleteBackup", namespace, 1, 3)

	return klioFeatures.NewIncompleteBackupFeature(klioFeatures.IncompleteBackupFeatureConfig{
		Name:        cfg.Name,
		Setup:       cfg.Setup,
		Teardown:    cfg.Teardown,
		Backups:     cfg.Backups[:3],
		KlioServer:  cfg.KlioServer,
		Namespace:   cfg.Namespace,
		ClusterName: cfg.ClusterName,
	})
}
