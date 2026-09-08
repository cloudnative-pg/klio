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

package v1alpha1

import (
	machineryapi "github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerMode defines the operation mode of the Server.
type ServerMode string

const (
	// ModeStandard corresponds to server with standard read/write permissions.
	ModeStandard ServerMode = "standard"
	// ModeReadOnly corresponds to a server with read-only permissions.
	ModeReadOnly ServerMode = "read-only"
)

// ServerSpec defines the desired state of Server.
// +kubebuilder:validation:XValidation:rule="self.mode == 'read-only' || has(self.tier1)",message="tier1 is required"
// +kubebuilder:validation:XValidation:rule="self.mode != 'read-only' || has(self.tier2)",message="tier2 is required when mode is read-only"
// +kubebuilder:validation:XValidation:rule="!(self.mode == 'read-only' && has(self.tier1))",message="tier1 cannot be set when mode is read-only"
// +kubebuilder:validation:XValidation:rule="!(self.mode == 'read-only' && has(self.tier2) && has(self.tier2.compression))",message="tier2.compression cannot be set when mode is read-only"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.storage) || !has(oldSelf.storage.pvcTemplate.resources) || !has(oldSelf.storage.pvcTemplate.resources.requests) || !('storage' in oldSelf.storage.pvcTemplate.resources.requests) || !('storage' in self.storage.pvcTemplate.resources.requests) || !quantity(self.storage.pvcTemplate.resources.requests['storage']).isLessThan(quantity(oldSelf.storage.pvcTemplate.resources.requests['storage']))",message="storage PVC size cannot be decreased"
type ServerSpec struct {
	// ImageConfiguration tells how to download the Klio
	// image.
	ImageConfiguration `json:",inline"`

	// TLSConfiguration is used for the server-side
	// certificate.
	TLSConfiguration `json:",inline"`

	// Mode selects the operation mode of the server.
	// +kubebuilder:validation:Enum=standard;read-only
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="mode is immutable"
	// +kubebuilder:default=standard
	Mode ServerMode `json:"mode"`

	// Tier1 is the Tier 1 configuration
	Tier1 *Tier1Configuration `json:"tier1,omitempty"`

	// Tier2 is the Tier 2 configuration
	Tier2 *Tier2Configuration `json:"tier2,omitempty"`

	// Storage is the configuration of the single PersistentVolumeClaim
	// mounted at /klio, hosting base backups, WAL, the work queue, and
	// the Tier 1/Tier 2 caches as fixed subdirectories (data, queue,
	// cache_tier1, cache_tier2).
	Storage Storage `json:"storage"`

	// Template to override the default StatefulSet of the Klio server.
	// WARNING: Modifying this template may break the server functionality if not done carefully.
	// This field is primarily intended for advanced configuration such as telemetry setup.
	// Use at your own risk and ensure thorough testing before applying changes.
	// +optional
	Template *PodTemplateSpec `json:"template,omitempty"`
}

// EmbeddedObjectMeta contains metadata for embedded objects.
type EmbeddedObjectMeta struct {
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PodTemplateSpec describes the data a pod should have when created from a template.
type PodTemplateSpec struct {
	// +optional
	Metadata EmbeddedObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec corev1.PodSpec `json:"spec,omitempty"`
}

// ToCoreV1 converts the custom PodTemplateSpec to corev1.PodTemplateSpec.
func (p *PodTemplateSpec) ToCoreV1() *corev1.PodTemplateSpec {
	if p == nil {
		return nil
	}

	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
		Spec: p.Spec,
	}
}

// ImageConfiguration contains the information needed to download
// the Klio image.
type ImageConfiguration struct {
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
}

// TLSConfiguration contains the information needed to configure
// the PKI infrastructure of the Klio server.
type TLSConfiguration struct {
	// TLSSecretName is the name of the Kubernetes secret containing the server-side certificate
	// to be used for the Klio server.
	TLSSecretName string `json:"tlsSecretName"`

	// ClientCASecretName is the name of the Kubernetes secret containing the CA certificate
	// to be used by the Klio server to validate the users.
	ClientCASecretName string `json:"caSecretName"`
}

// Storage defines the configuration for the Klio server's
// PersistentVolumeClaim.
type Storage struct {
	// PersistentVolumeClaimTemplate is used to generate the PVC that
	// backs the /klio directory tree for this server.
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`
}

// FileReference specifies a file from a volume source.
type FileReference struct {
	// Volume is the volume source to mount.
	Volume corev1.VolumeSource `json:"volume"`

	// Path is the file path within the mounted volume.
	Path string `json:"path"`
}

// FileSource specifies a source for a file. This wrapper allows future
// alternatives to be added without breaking the API.
// +kubebuilder:validation:ExactlyOneOf=fileReference
type FileSource struct {
	// FileReference specifies a file from a volume source.
	// +optional
	FileReference *FileReference `json:"fileReference,omitempty"`
}

// Tier1Configuration is the tier 1 configuration.
type Tier1Configuration struct {
	// EncryptionKeyFile specifies the Age-encrypted encryption key file.
	EncryptionKeyFile FileSource `json:"encryptionKeyFile"`

	// IdentityFile specifies the Age identity (private key) file used to
	// decrypt the encryption key.
	IdentityFile FileSource `json:"identityFile"`

	// Compression defines the repository-wide (global) compression policy
	// applied to base backups stored on tier1. Individual clusters can
	// override it through their PluginConfiguration.
	// +optional
	Compression *CompressionPolicy `json:"compression,omitempty"`
}

// Tier2Configuration is the tier 2 configuration.
type Tier2Configuration struct {
	// S3 contains the configuration parameters for an S3-based tier 2.
	S3 *S3Configuration `json:"s3"`

	// EncryptionKeyFile specifies the Age-encrypted encryption key file.
	EncryptionKeyFile FileSource `json:"encryptionKeyFile"`

	// IdentityFile specifies the Age identity (private key) file used to
	// decrypt the encryption key.
	IdentityFile FileSource `json:"identityFile"`

	// Compression defines the repository-wide (global) compression policy
	// applied to base backups stored on tier2. Individual clusters can
	// override it through their PluginConfiguration.
	// +optional
	Compression *CompressionPolicy `json:"compression,omitempty"`
}

// S3Configuration is the configuration to a S3 defined tier 2.
type S3Configuration struct {
	// BucketName is the name of the bucket
	BucketName string `json:"bucketName"`

	// Prefix is the path within the bucket under which all Klio objects
	// are stored, allowing a single bucket to be shared across multiple deployments.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// Endpoint is the endpoint to be used
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Region is the region to be used
	// +optional
	Region string `json:"region,omitempty"`

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

// GetStatefulSetName returns the name of the StatefulSet running the Klio server.
func (s *Server) GetStatefulSetName() string {
	return s.Name + "-klio"
}

// GetServiceName returns the name of the service associated with the Klio server.
func (s *Server) GetServiceName() string {
	return s.Name
}
