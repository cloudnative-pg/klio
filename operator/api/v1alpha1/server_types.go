package v1alpha1

import (
	machineryapi "github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerSpec defines the desired state of Server.
// +kubebuilder:validation:XValidation:rule="!has(self.tier2) || has(self.queueConfiguration)",message="queueConfiguration is required when tier2 is defined"
type ServerSpec struct {
	// BaseConfiguration is the configuration of the Kopia server
	// +optional
	BaseConfiguration BaseConfiguration `json:"baseConfiguration,omitzero"`

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

	// ClientCASecretName is the name of the Kubernetes secret containing the CA certificate
	// to be used by the Klio server to validate the users.
	ClientCASecretName string `json:"caSecretName"`

	// Resources defines the resource requirements for the Klio server
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitzero"`

	// CacheConfiguration is the configuration of the PVC that should be
	// used for the cache
	CacheConfiguration CacheConfiguration `json:"cacheConfiguration"`

	// DataConfiguration is the configuration of the PVC that should be used
	// for the base backups
	DataConfiguration DataConfiguration `json:"dataConfiguration"`

	// QueueConfiguration is the configuration of the PVC that should host
	// the task queue.
	// +optional
	QueueConfiguration *QueueConfiguration `json:"queueConfiguration,omitempty"`

	// Password is a reference to a secret containing the Klio password
	Password *machineryapi.SecretKeySelector `json:"password"`

	// Tier2 is the Tier 2 configuration
	Tier2 *Tier2Configuration `json:"tier2,omitempty"`

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
	Resources corev1.ResourceRequirements `json:"resources,omitzero"`
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

// QueueConfiguration defines the configuration for the directory hosting the
// task queue.
type QueueConfiguration struct {
	// QueueResources defines the resource requirements for the NATS server
	// +optional
	QueueResources corev1.ResourceRequirements `json:"resources,omitzero"`

	// PersistentVolumeClaimTemplate is used to generate the configuration for
	// the PVC hosting the work queue.
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`
}

// Tier2Configuration is the tier 2 configuration.
type Tier2Configuration struct {
	// S3 contains the configuration parameters for an S3-based tier 2
	S3 *S3Configuration `json:"s3"`
}

// S3Configuration is the configuration to a S3 defined tier 2.
type S3Configuration struct {
	// BucketName is the name of the bucket
	BucketName string `json:"bucketName"`

	// Prefix is the prefix to be used for the stored files
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// Endpoint is the endpoint to be used
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Region is the region to be used
	// +optional
	Region string `json:"region,omitempty"`

	// WALEncryptionPassword is a pointer to the key in a secret containing the encryption password.
	WALEncryptionPassword *machineryapi.SecretKeySelector `json:"walEncryptionPassword"`

	// The S3 access key ID
	// +optional
	AccessKeyID *machineryapi.SecretKeySelector `json:"accessKeyId,omitempty"`

	// The S3 access key
	// +optional
	SecretAccessKey *machineryapi.SecretKeySelector `json:"secretAccessKey,omitempty"`

	// The S3 session token
	// +optional
	SessionToken *machineryapi.SecretKeySelector `json:"sessionToken,omitempty"`

	// A pointer to a custom CA bundle
	// +optional
	CustomCABundle *machineryapi.SecretKeySelector `json:"customCaBundle,omitempty"`
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

	Spec ServerSpec `json:"spec"`
	// +optional
	Status ServerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ServerList contains a list of Server.
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

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
