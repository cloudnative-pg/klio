package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PluginConfigurationSpec defines the desired state of client configuration.
// +kubebuilder:validation:AtMostOneOf=backupRef;backupId
type PluginConfigurationSpec struct {
	// ServerAddress is the address of the Klio server in the format host:port or host
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServerAddress string `json:"serverAddress"`

	// ClientSecretName is the name of the secret containing the client credentials
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientSecretName string `json:"clientSecretName"`

	// ServerSecretName is the name of the secret containing the server TLS certificate
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServerSecretName string `json:"serverSecretName"`

	// ClusterName is the name of the PostgreSQL cluster we are connecting to
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// BackupRef is the reference to the backup we should use for restores
	// +optional
	BackupRef string `json:"backupRef,omitempty"`

	// BackupID is the ID of the backup we should use for restores
	// +optional
	BackupID string `json:"backupId,omitempty"`

	// Pprof enables the pprof endpoint for performance profiling
	// +optional
	Pprof bool `json:"pprof,omitempty"`

	// RetentionPolicy defines how many backups we should keep
	// +optional
	RetentionPolicy *RetentionPolicy `json:"retention,omitempty" mapstructure:"retention"`
}

// RetentionPolicy defines how many backups we should keep.
type RetentionPolicy struct {
	// KeepLatest is the number of latest backups to keep
	// optional
	KeepLatest *int `json:"keepLatest,omitempty" mapstructure:"keepLatest"`

	// KeepAnnual is the number of annual backups to keep
	// optional
	KeepAnnual *int `json:"keepAnnual,omitempty" mapstructure:"keepAnnual"`

	// KeepMonthly is the number of monthly backups to keep
	// optional
	KeepMonthly *int `json:"keepMonthly,omitempty" mapstructure:"keepMonthly"`

	// KeepWeekly is the number of weekly backups to keep
	// optional
	KeepWeekly *int `json:"keepWeekly,omitempty" mapstructure:"keepWeekly"`

	// KeepDaily is the number of daily backups to keep
	// optional
	KeepDaily *int `json:"keepDaily,omitempty" mapstructure:"keepDaily"`

	// KeepHourly is the number of hourly backups to keep
	// optional
	KeepHourly *int `json:"keepHourly,omitempty" mapstructure:"keepHourly"`
}

// PluginConfigurationStatus defines the observed state of ClientConfig.
type PluginConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PluginConfiguration is the Schema for the client configuration API.
type PluginConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec PluginConfigurationSpec `json:"spec"`
	// +optional
	Status PluginConfigurationStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PluginConfigurationList contains a list of PluginConfiguration.
type PluginConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []PluginConfiguration `json:"items"`
}

//nolint:gochecknoinits
func init() {
	SchemeBuilder.Register(&PluginConfiguration{}, &PluginConfigurationList{})
}
