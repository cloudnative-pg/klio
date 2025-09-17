package v1alpha1

import (
	machineryapi "github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerSpec defines the desired state of Server.
type ServerSpec struct {
	// BaseConfiguration is the configuration of the Kopia server
	// +optional
	BaseConfiguration BaseConfiguration `json:"baseConfiguration,omitempty"`

	// Image is the image to be used for the Klio server
	Image string `json:"image"`

	// ImagePullPolicy defines the policy for pulling the image
	// +optional
	// +kubebuilder:default=IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the
	// images
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// TLSSecretName is the name of the Kubernetes secret containing the server-side certificate
	// to be used for the Klio server.
	TLSSecretName string `json:"tlsSecretName"`

	// Resources defines the resource requirements for the Klio server
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// CacheConfiguration is the configuration of the PVC that should be
	// used for the cache
	CacheConfiguration CacheConfiguration `json:"cacheConfiguration"`

	// DataConfiguration is the configuration of the PVC that should be used
	// for the base backups
	DataConfiguration DataConfiguration `json:"dataConfiguration"`

	// Password is a reference to a secret containing the Klio password
	Password *machineryapi.SecretKeySelector `json:"password"`

	// Users is a reference to a secret containing a htpasswd file at the 'htpasswd' key.
	Users corev1.LocalObjectReference `json:"users"`

	// Template to override the default StatefulSet of the Klio server.
	// WARNING: Modifying this template may break the server functionality if not done carefully.
	// This field is primarily intended for advanced configuration such as telemetry setup.
	// Use at your own risk and ensure thorough testing before applying changes.
	// +optional
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`
}

// BaseConfiguration defines the configuration for the base server.
type BaseConfiguration struct {
	// Resources defines the resource requirements for the Kopia server
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// AdminUser is a reference to a secret of type 'kubernetes.io/basic-auth'
	// +optional
	AdminUser corev1.LocalObjectReference `json:"adminUser,omitempty"`
}

// DataConfiguration defines the configuration for the data directory.
type DataConfiguration struct {
	// Template to be used to generate the Persistent Volume Claim needed for the data folder,
	// containing base backups and WAL files.
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`
}

// CacheConfiguration defines the configuration for the cache directory.
type CacheConfiguration struct {
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`
}

// ServerStatus defines the observed state of Server.
type ServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Server is the Schema for the servers API.
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   ServerSpec   `json:"spec"`
	Status ServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerList contains a list of Server.
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Server `json:"items"`
}

//nolint:gochecknoinits
func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}

// GetServiceName returns the name of the service associated with the Klio server.
func (s *Server) GetServiceName() string {
	return s.Name
}
