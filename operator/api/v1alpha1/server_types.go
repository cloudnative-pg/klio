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
	// ServerTLSIdentity is the TLS identity presented to clients: the
	// server certificate and its matching private key.
	// +kubebuilder:validation:Required
	ServerTLSIdentity TLSIdentity `json:"serverTlsIdentity"`

	// ClientCA is the CA bundle used to verify client certificates.
	// It must contain the PEM-encoded CA certificate(s).
	// +kubebuilder:validation:Required
	ClientCA VolumeFileReference `json:"clientCa"`
}

// Storage defines the configuration for the Klio server's
// PersistentVolumeClaim.
type Storage struct {
	// PersistentVolumeClaimTemplate is used to generate the PVC that
	// backs the /klio directory tree for this server.
	PersistentVolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate"`
}

// VolumeSource is the subset of volume sources Klio can read files from.
// +kubebuilder:validation:ExactlyOneOf=secret;configMap;projected;csi
// +kubebuilder:validation:XValidation:rule="!has(self.secret) || (has(self.secret.secretName) && size(self.secret.secretName) > 0)",message="secret.secretName must be set when the secret volume source is used"
// +kubebuilder:validation:XValidation:rule="!has(self.configMap) || (has(self.configMap.name) && size(self.configMap.name) > 0)",message="configMap.name must be set when the configMap volume source is used"
// +kubebuilder:validation:XValidation:rule="!has(self.projected) || (has(self.projected.sources) && size(self.projected.sources) > 0)",message="projected.sources must not be empty when the projected volume source is used"
// +kubebuilder:validation:XValidation:rule="!has(self.csi) || (has(self.csi.driver) && size(self.csi.driver) > 0)",message="csi.driver must be set when the csi volume source is used"
// +kubebuilder:validation:XValidation:rule="!has(self.secret) || !has(self.secret.optional) || !self.secret.optional",message="secret.optional must be unset or false, credential volumes cannot be optional"
// +kubebuilder:validation:XValidation:rule="!has(self.configMap) || !has(self.configMap.optional) || !self.configMap.optional",message="configMap.optional must be unset or false, credential volumes cannot be optional"
type VolumeSource struct {
	// Secret is a volume populated by a Secret.
	// +optional
	Secret *corev1.SecretVolumeSource `json:"secret,omitempty"`

	// ConfigMap is a volume populated by a ConfigMap.
	// +optional
	ConfigMap *corev1.ConfigMapVolumeSource `json:"configMap,omitempty"`

	// Projected is a projected volume (secrets, config maps, cluster
	// trust bundles, ...).
	// +optional
	Projected *corev1.ProjectedVolumeSource `json:"projected,omitempty"`

	// CSI is a volume provided by a CSI driver.
	// +optional
	CSI *corev1.CSIVolumeSource `json:"csi,omitempty"`
}

// ToCoreV1 converts the subset to a corev1.VolumeSource. The result is a
// deep copy, so callers may mutate it.
func (s *VolumeSource) ToCoreV1() corev1.VolumeSource {
	c := s.DeepCopy()

	return corev1.VolumeSource{
		Secret:    c.Secret,
		ConfigMap: c.ConfigMap,
		Projected: c.Projected,
		CSI:       c.CSI,
	}
}

// VolumeFileReference specifies a file from a volume source.
type VolumeFileReference struct {
	// Volume is the volume source to mount.
	Volume VolumeSource `json:"volume"`

	// Path is the file path within the mounted volume.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:validation:XValidation:rule="!self.startsWith('/') && !self.split('/').exists(s, s == '..')",message="must be a relative path without '..' segments"
	Path string `json:"path"`
}

// TLSIdentity is the certificate and private key a component uses to
// authenticate itself over TLS.
type TLSIdentity struct {
	// Volume is the volume source that holds the certificate and the
	// private key.
	Volume VolumeSource `json:"volume"`

	// CertPath is the path of the certificate file within the volume.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:validation:XValidation:rule="!self.startsWith('/') && !self.split('/').exists(s, s == '..')",message="must be a relative path without '..' segments"
	CertPath string `json:"certPath"`

	// KeyPath is the path of the private key file within the volume.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:validation:XValidation:rule="!self.startsWith('/') && !self.split('/').exists(s, s == '..')",message="must be a relative path without '..' segments"
	KeyPath string `json:"keyPath"`
}

// Tier1Configuration is the tier 1 configuration.
type Tier1Configuration struct {
	// EncryptionKeyFile specifies the Age-encrypted encryption key file.
	EncryptionKeyFile VolumeFileReference `json:"encryptionKeyFile"`

	// IdentityFile specifies the Age identity (private key) file used to
	// decrypt the encryption key.
	IdentityFile VolumeFileReference `json:"identityFile"`

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
	EncryptionKeyFile VolumeFileReference `json:"encryptionKeyFile"`

	// IdentityFile specifies the Age identity (private key) file used to
	// decrypt the encryption key.
	IdentityFile VolumeFileReference `json:"identityFile"`

	// Compression defines the repository-wide (global) compression policy
	// applied to base backups stored on tier2. Individual clusters can
	// override it through their PluginConfiguration.
	// +optional
	Compression *CompressionPolicy `json:"compression,omitempty"`
}

// S3Configuration is the configuration to a S3 defined tier 2.
// +kubebuilder:validation:XValidation:rule="!has(self.credentialsFile) || !(has(self.accessKeyId) || has(self.secretAccessKey) || has(self.sessionToken))",message="credentialsFile cannot be combined with accessKeyId, secretAccessKey or sessionToken"
// +kubebuilder:validation:XValidation:rule="!has(self.profile) || has(self.credentialsFile)",message="profile requires credentialsFile"
// +kubebuilder:validation:XValidation:rule="has(self.accessKeyId) == has(self.secretAccessKey)",message="accessKeyId and secretAccessKey must be set together"
// +kubebuilder:validation:XValidation:rule="!has(self.sessionToken) || (has(self.accessKeyId) && has(self.secretAccessKey))",message="sessionToken requires accessKeyId and secretAccessKey"
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

	// CredentialsFile is an AWS shared credentials file (INI format) mounted
	// from a volume. It is mutually exclusive with accessKeyId,
	// secretAccessKey and sessionToken.
	// +optional
	CredentialsFile *VolumeFileReference `json:"credentialsFile,omitempty"`

	// Profile is the profile to use within CredentialsFile.
	// +optional
	Profile string `json:"profile,omitempty"`

	// CustomCABundle is a PEM-encoded CA bundle, mounted from a volume, that
	// is trusted when connecting to the S3 endpoint.
	// +optional
	CustomCABundle *VolumeFileReference `json:"customCaBundle,omitempty"`
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
