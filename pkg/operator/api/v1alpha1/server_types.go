package v1alpha1

import (
	machineryapi "github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServerSpec defines the desired state of Server.
type ServerSpec struct {
	KopiaConfiguration KopiaConfiguration `json:"kopiaConfiguration"`

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

	TLSSecretName string `json:"tlsSecretName"`

	// Resources defines the resource requirements for the Klio server
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	LogConfiguration LogConfiguration `json:"logConfiguration"`

	CacheConfiguration CacheConfiguration `json:"cacheConfiguration"`

	DataConfiguration DataConfiguration `json:"dataConfiguration"`

	// Password is a reference to a secret containing the Klio password
	Password *machineryapi.SecretKeySelector `json:"password"`
}

// KopiaConfiguration defines the configuration for the Kopia server
type KopiaConfiguration struct {
	// Resources defines the resource requirements for the Kopia server
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// AdminUser is a reference to a secret of type 'kubernetes.io/basic-auth'
	AdminUser corev1.LocalObjectReference `json:"adminUser"`
}

// DataConfiguration defines the configuration for the data directory
type DataConfiguration struct {
	// Template to be used to generate the Persistent Volume Claim needed for data folder
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`
}

// LogConfiguration defines the configuration for the logs directory
type LogConfiguration struct {
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`

	// KopiasLogDirectory specifies where Kopia logs should be stored. The volume will be mounted under /logs
	// +kubebuilder:default="/kopia"
	KopiaLogsDirectory string `json:"kopiaLogsDirectory"`
}

// CacheConfiguration defines the configuration for the cache directory
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
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}
