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

	TLSSecretName string `json:"tlsSecretName"`

	// Password is a reference to a secret containing the Klio password
	Password *machineryapi.SecretKeySelector `json:"password"`

	// Template to be used to generate the Persistent Volume Claim needed for data folder
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`
}

// KopiaConfiguration defines the configuration for the Kopia server
type KopiaConfiguration struct {
	// ConfigPath specifies where the Kopia configuration file should be stored
	// +optional
	// +kubebuilder:default="/data/kopia.config"
	ConfigPath string `json:"configPath,omitempty"`

	// LogDirectory specifies where Kopia logs should be stored
	// +optional
	// +kubebuilder:default="/data/kopia_logs"
	LogDirectory string `json:"logDirectory,omitempty"`

	// CacheDirectory specifies where Kopia cache should be stored
	// +optional
	// +kubebuilder:default="/data/kopia_cache"
	CacheDirectory string `json:"cacheDirectory,omitempty"`

	// User specifies the username to run Kopia as
	// +optional
	// +kubebuilder:default="kopia"
	User string `json:"user,omitempty"`

	Password *machineryapi.SecretKeySelector `json:"password"`
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
