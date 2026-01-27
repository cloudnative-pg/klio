package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PluginConfigurationSpec defines the desired state of client configuration.
// +kubebuilder:validation:XValidation:rule="!self.readOnly || (has(self.tier2) && (!has(self.tier2.enableBackup) || !self.tier2.enableBackup) && self.tier2.enableRecovery && !has(self.tier1))",message="when readOnly is true, tier2.enableRecovery must be true, tier2.enableBackup must be false, and tier1 must not exist"
type PluginConfigurationSpec struct {
	// ServerAddress is the address of the Klio server
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServerAddress string `json:"serverAddress"`

	// Tier1 is the Tier 1 configuration
	// +optional
	Tier1 Tier1PluginConfiguration `json:"tier1,omitzero"`

	// Tier2 is the Tier 2 configuration
	// +optional
	Tier2 Tier2PluginConfiguration `json:"tier2,omitzero"`

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

	// Pprof enables the pprof endpoint for performance profiling
	// +optional
	Pprof bool `json:"pprof,omitempty"`

	// ReadOnly is true when you can only read from the server.
	// In this case, tier1 should not be configured.
	// +kubebuilder:default=false
	ReadOnly bool `json:"readOnly"`

	// Containers allows defining a list of containers that will be merged with the Klio sidecar containers.
	// This enables users to customize the sidecars with additional environment variables, volume mounts,
	// resource limits, and other container settings without polluting the PostgreSQL container environment.
	//
	// Merge behavior:
	// - Containers are matched by name (klio-plugin, klio-wal, klio-restore)
	// - User customizations serve as the base
	// - Klio required values (name, args, CONTAINER_NAME env var) always override user values
	// - User-defined environment variables and volume mounts are preserved
	// - Template defaults are applied only for fields not set by the user or Klio
	//
	// +optional
	// +kubebuilder:validation:MaxItems=3
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:XValidation:rule="self.all(c, c.name in ['klio-plugin', 'klio-wal', 'klio-restore'])",message="container name must be one of: klio-plugin, klio-wal, klio-restore"
	Containers []corev1.Container `json:"containers,omitempty"`
}

// Tier1PluginConfiguration configures tier1 backup and recovery settings.
type Tier1PluginConfiguration struct {
	// RetentionPolicy defines how many backups we should keep
	// +optional
	RetentionPolicy *RetentionPolicy `json:"retention,omitempty" mapstructure:"retention"`
}

// Tier2PluginConfiguration configures tier2 backup and recovery settings.
// +kubebuilder:validation:XValidation:rule="self.enableBackup || self.enableRecovery",message="at least one of enableBackup or enableRecovery must be true"
type Tier2PluginConfiguration struct {
	// EnableBackup controls whether WAL and base backups should be stored in tier2
	// +optional
	EnableBackup bool `json:"enableBackup,omitempty"`

	// EnableRecovery controls whether tier2 should be included in the recovery source list
	// +optional
	EnableRecovery bool `json:"enableRecovery,omitempty"`

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
