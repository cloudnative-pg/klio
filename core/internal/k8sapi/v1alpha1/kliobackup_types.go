package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KlioBackupSpec defines the desired state of a KlioBackup.
type KlioBackupSpec struct {
	// ClusterName is the name of the cluster that has been backed up
	ClusterName string `json:"clusterName"`
	// BackupID is the unique identifier of the backup
	BackupID string `json:"backupID"`
}

// KlioBackupStatus defines the observed state of a KlioBackup.
type KlioBackupStatus struct {
	// StartLSN is the LSN of the backup start
	StartLSN uint64 `json:"startLSN"`

	// EndLSN is the LSN of the backup end
	EndLSN uint64 `json:"endLSN"`

	// StartWAL is the current WAL when the backup started
	StartWAL string `json:"startWAL"`

	// EndWAL is the current WAL when the backup ends
	EndWAL string `json:"endWAL"`

	// Tablespaces are the metadata of the tablespaces
	Tablespaces TablespaceLayoutList `json:"tablespaces,omitempty"`

	// Annotations is a generic data store where each
	// backend can put its metadata.
	Annotations map[string]string `json:"annotations,omitempty"`

	// StartedAt is the current time when the backup started.
	StartedAt metav1.Time `json:"startedAt"`

	// StoppedAt is the current time when the backup ended.
	StoppedAt metav1.Time `json:"stoppedAt"`
}

// TablespaceLayoutList is a list of TablespaceLayout.
type TablespaceLayoutList []TablespaceLayout

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

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KlioBackup is the Schema for a Klio Backup API.
type KlioBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec KlioBackupSpec `json:"spec"`
	// +optional
	Status KlioBackupStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KlioBackupList contains a list of KlioBackup.
type KlioBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"` //nolint:modernize

	Items []KlioBackup `json:"items"`
}

//nolint:gochecknoinits
func init() {
	SchemeBuilder.Register(&KlioBackup{}, &KlioBackupList{})
}
