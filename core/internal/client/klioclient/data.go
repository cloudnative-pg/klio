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

// TablespaceLayout is the on-disk structure of a tablespace.
type TablespaceLayout struct {
	// Name is the tablespace name
	Name string `json:"name"`

	// Oid is the OID of the tablespace.
	Oid string `json:"oid"`

	// Path is the path where the tablespace can be found.
	Path string `json:"path"`

	// Annotations is a generic data store where each backend
	// can annotate its metadata.
	Annotations map[string]string `json:"annotations"`
}

// BackupMetadata is the metadata to be stored with set of backup snapshots.
type BackupMetadata struct {
	// Name is the backup name
	Name string `json:"name"`

	// ClusterName is the name of the cluster that was backed up
	ClusterName string `json:"clusterName"`

	// StartLSN is the LSN of the backup start
	StartLSN uint64 `json:"startLsn"`

	// EndLSN is the LSN of the backup end
	EndLSN uint64 `json:"endLsn"`

	// StartWAL is the current WAL when the backup started
	StartWAL string `json:"startWal"`

	// EndWAL is the current WAL when the backup ends
	EndWAL string `json:"endWal"`

	// Timeline is the backup timeline
	Timeline int `json:"tli"`

	// SegmentSize is the segment size of the WALs during the backup.
	SegmentSize uint64 `json:"segmentSize"`

	// BackupLabel is the backup label content
	BackupLabel string `json:"backupLabel"`

	// TablespaceMap is the tablespace map content
	TablespaceMap string `json:"tablespaceMap"`

	// PgData is the data directory location
	PgData string `json:"pgData"`

	// Tablespaces are the metadata of the tablespaces
	Tablespaces []TablespaceLayout `json:"tablespaces,omitempty"`

	// Annotations is a generic data store where each
	// backend can put its metadata.
	Annotations map[string]string `json:"annotations,omitempty"`

	// StartedAt is the current time when the backup started.
	StartedAt int64 `json:"startedAt"`

	// StoppedAt is the current time when the backup ended.
	StoppedAt int64 `json:"stoppedAt"`
}

// BackupList is a list of backups.
type BackupList []BackupMetadata

// SetAnnotation sets an annotation on a backup metadata to the
// specified value.
func (m *BackupMetadata) SetAnnotation(n, v string) {
	if m.Annotations == nil {
		m.Annotations = make(map[string]string)
	}

	m.Annotations[n] = v
}
