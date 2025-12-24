package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RecoverySourceSpec defines a remote Klio tier2 to be used as a
// recovery source.
type RecoverySourceSpec struct {
	// ImageConfiguration tells how to download the Klio
	// image.
	ImageConfiguration `json:",inline"`

	// TLSConfiguration is used for the server-side
	// certificate.
	TLSConfiguration `json:",inline"`

	// Tier2 is the tier 2 configuration to be used by this recovery source.
	Tier2 Tier2Configuration `json:"tier2"`

	// Storage is the storage resources to be used
	// for this Klio recovery source.
	Storage RecoverySourceStorageConfiguration `json:"storage"`

	// Template to override the default StatefulSet of the Klio recovery source.
	// WARNING: Modifying this template may break the server functionality if not done carefully.
	// This field is primarily intended for advanced configuration such as telemetry setup.
	// Use at your own risk and ensure thorough testing before applying changes.
	// +optional
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`
}

// RecoverySourceStorageConfiguration defines the storage
// to be used for this recovery source.
type RecoverySourceStorageConfiguration struct {
	// Cache is the configuration of the PVC that should be
	// used for the cache.
	Cache CacheConfiguration `json:"cache"`
}

// RecoverySourceStatus defines the observed state of recovery source.
type RecoverySourceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RecoverySource is the Schema for the recovery source API.
type RecoverySource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec RecoverySourceSpec `json:"spec"`
	// +optional
	Status RecoverySourceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// RecoverySourceList contains a list of RecoverySources.
type RecoverySourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []RecoverySource `json:"items"`
}

//nolint:gochecknoinits
func init() {
	SchemeBuilder.Register(&RecoverySource{}, &RecoverySourceList{})
}

// GetServiceName returns the name of the service associated with the Klio server.
func (s *RecoverySource) GetServiceName() string {
	return s.Name
}
