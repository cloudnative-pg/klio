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

package klioclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyTier1IncompleteBackups(t *testing.T) {
	got := ClassifyTier1IncompleteBackups([]IncompleteBackup{
		{ClusterName: "c1", BackupName: "b1"},
		{ClusterName: "c1", BackupName: "b2", Latest: true},
		// The metadata is uploaded last: with it present, the latest attempt
		// is not uploading, it lost a part.
		{ClusterName: "c2", BackupName: "b1", Latest: true, MetadataPresent: true},
	})

	assert.Equal(t, []IncompleteBackup{
		{ClusterName: "c1", BackupName: "b1", Tier: "tier1", Status: IncompleteStatusUnrecoverable},
		{ClusterName: "c1", BackupName: "b2", Tier: "tier1", Status: IncompleteStatusPending, Latest: true},
		{
			ClusterName: "c2", BackupName: "b1", Tier: "tier1", Status: IncompleteStatusUnrecoverable,
			Latest: true, MetadataPresent: true,
		},
	}, got)
}

func TestClassifyTier2IncompleteBackups(t *testing.T) {
	tier1Complete := BackupList{{ClusterName: "c1", Name: "b1"}}

	got := ClassifyTier2IncompleteBackups([]IncompleteBackup{
		{ClusterName: "c1", BackupName: "b1", Latest: true},
		{ClusterName: "c1", BackupName: "b2"},
		{ClusterName: "c2", BackupName: "b1"},
	}, tier1Complete)

	assert.Equal(t, []IncompleteBackup{
		{ClusterName: "c1", BackupName: "b1", Tier: "tier2", Status: IncompleteStatusRecoverable, Latest: true},
		{ClusterName: "c1", BackupName: "b2", Tier: "tier2", Status: IncompleteStatusUnrecoverable},
		{ClusterName: "c2", BackupName: "b1", Tier: "tier2", Status: IncompleteStatusUnrecoverable},
	}, got)
}
