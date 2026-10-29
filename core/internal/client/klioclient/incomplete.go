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

// IncompleteStatus says whether an incomplete backup can still complete.
type IncompleteStatus string

const (
	// IncompleteStatusPending is a tier1 backup without metadata that is
	// the latest attempt of its cluster: it may still be uploading, or be
	// a dead attempt.
	IncompleteStatusPending IncompleteStatus = "PENDING"

	// IncompleteStatusRecoverable is a tier2 backup that is complete on
	// tier1: the next relay of its cluster rebuilds it.
	IncompleteStatusRecoverable IncompleteStatus = "RECOVERABLE"

	// IncompleteStatusUnrecoverable is a backup that can never complete.
	IncompleteStatusUnrecoverable IncompleteStatus = "UNRECOVERABLE"
)

// Tier names used in IncompleteBackup.
const (
	Tier1Name = "tier1"
	Tier2Name = "tier2"
)

// IncompleteBackup is a backup not fully present on a tier.
type IncompleteBackup struct {
	ClusterName string           `json:"clusterName"`
	BackupName  string           `json:"backupName"`
	Tier        string           `json:"tier"`
	Status      IncompleteStatus `json:"status"`
	StartedAt   string           `json:"startedAt"`

	// MetadataPresent reports whether the metadata snapshot exists. The
	// metadata is uploaded last, so a backup without it was interrupted.
	MetadataPresent bool `json:"metadataPresent"`

	// MissingParts lists the parts the metadata expects that have no
	// snapshot. Empty when the metadata itself is missing.
	MissingParts []string `json:"missingParts"`

	// Latest reports whether this is the latest started backup of its
	// cluster on its tier. It only feeds the status, so it is not part of
	// the JSON output.
	Latest bool `json:"-"`
}

// ClassifyTier1IncompleteBackups sets the tier and status of backups
// listed on tier1. Backup names are per attempt and the metadata is
// uploaded last, so only the latest attempt of a cluster, and only
// without its metadata, can still be uploading.
func ClassifyTier1IncompleteBackups(backups []IncompleteBackup) []IncompleteBackup {
	for i := range backups {
		backups[i].Tier = Tier1Name
		backups[i].Status = IncompleteStatusUnrecoverable
		if backups[i].Latest && !backups[i].MetadataPresent {
			backups[i].Status = IncompleteStatusPending
		}
	}

	return backups
}

// ClassifyTier2IncompleteBackups sets the tier and status of backups
// listed on tier2. The relay copies to tier2 whatever a cluster has on
// tier1, so a backup complete on tier1 is rebuilt by the next relay.
func ClassifyTier2IncompleteBackups(backups []IncompleteBackup, tier1Complete BackupList) []IncompleteBackup {
	type clusterBackup struct{ cluster, name string }

	relayable := make(map[clusterBackup]struct{}, len(tier1Complete))
	for _, b := range tier1Complete {
		relayable[clusterBackup{b.ClusterName, b.Name}] = struct{}{}
	}

	for i := range backups {
		backups[i].Tier = Tier2Name
		backups[i].Status = IncompleteStatusUnrecoverable
		if _, ok := relayable[clusterBackup{backups[i].ClusterName, backups[i].BackupName}]; ok {
			backups[i].Status = IncompleteStatusRecoverable
		}
	}

	return backups
}
