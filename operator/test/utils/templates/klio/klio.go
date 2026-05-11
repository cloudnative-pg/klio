package klio

import (
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// Tier2S3Options contains the S3 configuration parameters for tier2 storage.
type Tier2S3Options struct {
	// S3BucketName is the name of the S3 bucket.
	S3BucketName string
	// S3Prefix is the prefix for S3 objects.
	S3Prefix string
	// S3Endpoint is the S3 endpoint URL.
	S3Endpoint string
	// S3Region is the S3 region.
	S3Region string
	// S3AccessKeySecretName is the secret name containing the S3 access key.
	S3AccessKeySecretName string
	// S3SecretKeySecretName is the secret name containing the S3 secret key.
	S3SecretKeySecretName string
	// S3CABundleSecretName is the secret name containing the S3 CA bundle.
	S3CABundleSecretName string
}

// EncryptionOptions contains the secret names for encryption key and identity files.
type EncryptionOptions struct {
	// EncryptionKeySecretName is the secret containing the Age-encrypted key file.
	EncryptionKeySecretName string
	// EncryptionKeyFileName is the key within the secret.
	EncryptionKeyFileName string
	// IdentitySecretName is the secret containing the Age identity file.
	IdentitySecretName string
	// IdentityFileName is the key within the secret.
	IdentityFileName string
}

func newFileSource(secretName, fileName string) kliov1alpha1.FileSource {
	return kliov1alpha1.FileSource{
		FileReference: &kliov1alpha1.FileReference{
			Volume: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
			Path: fileName,
		},
	}
}

// BuildTier2Configuration creates a Tier2Configuration from S3 options, encryption options,
// and a storage class name. If storageClass is empty, the cluster's default storage class is used.
func BuildTier2Configuration(
	s3Opts Tier2S3Options,
	encOpts EncryptionOptions,
	storageClass string,
) kliov1alpha1.Tier2Configuration {
	var sc *string
	if storageClass != "" {
		sc = new(storageClass)
	}

	return kliov1alpha1.Tier2Configuration{
		Cache: kliov1alpha1.Cache{
			PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
				StorageClassName: sc,
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
		S3: &kliov1alpha1.S3Configuration{
			BucketName: s3Opts.S3BucketName,
			Prefix:     s3Opts.S3Prefix,
			Endpoint:   s3Opts.S3Endpoint,
			Region:     s3Opts.S3Region,
			AccessKeyID: &cnpgv1.SecretKeySelector{
				LocalObjectReference: api.LocalObjectReference{
					Name: s3Opts.S3AccessKeySecretName,
				},
				Key: "RUSTFS_ACCESS_KEY",
			},
			SecretAccessKey: &cnpgv1.SecretKeySelector{
				LocalObjectReference: api.LocalObjectReference{
					Name: s3Opts.S3SecretKeySecretName,
				},
				Key: "RUSTFS_SECRET_KEY",
			},
			CustomCABundle: &cnpgv1.SecretKeySelector{
				LocalObjectReference: api.LocalObjectReference{
					Name: s3Opts.S3CABundleSecretName,
				},
				Key: "ca.crt",
			},
		},
		EncryptionKeyFile: newFileSource(encOpts.EncryptionKeySecretName, encOpts.EncryptionKeyFileName),
		IdentityFile:      newFileSource(encOpts.IdentitySecretName, encOpts.IdentityFileName),
	}
}

// ServerTemplateOptions are the options needed to create a Klio server.
type ServerTemplateOptions struct {
	// Image is the Klio server container image. If empty, the default
	// test image is used.
	Image string

	// StorageClass is the Kubernetes storage class used for all PVC templates
	// (tier1 cache, tier1 data, queue). If empty, the cluster's default
	// storage class is used.
	StorageClass string

	// ImagePullSecret is the name of the Kubernetes secret used to pull the
	// Klio server image. If empty, no pull secret is configured.
	ImagePullSecret string

	// TLSSecretName is the secret to be used to expose the Klio server.
	TLSSecretName string

	// ClientCASecretName is the secret that will be used by Kopia and by
	// the Klio WAL server to authenticate users.
	ClientCASecretName string

	// Encryption contains the encryption key and identity file options.
	Encryption EncryptionOptions
}

// newBaseServer creates a server with common fields (metadata, image, TLS) but no tier configuration.
func newBaseServer(name, namespace string, opts ServerTemplateOptions) *kliov1alpha1.Server {
	imgCfg := kliov1alpha1.ImageConfiguration{
		Image:           opts.Image,
		ImagePullPolicy: corev1.PullAlways,
	}
	if opts.ImagePullSecret != "" {
		imgCfg.ImagePullSecrets = []corev1.LocalObjectReference{{Name: opts.ImagePullSecret}}
	}

	return &kliov1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kliov1alpha1.ServerSpec{
			ImageConfiguration: imgCfg,
			TLSConfiguration: kliov1alpha1.TLSConfiguration{
				TLSSecretName:      opts.TLSSecretName,
				ClientCASecretName: opts.ClientCASecretName,
			},
		},
	}
}

// GetServerObject returns a Klio server Object with tier1 and queue configuration.
func GetServerObject(
	name,
	namespace string,
	opts ServerTemplateOptions,
) *kliov1alpha1.Server {
	var sc *string
	if opts.StorageClass != "" {
		sc = new(opts.StorageClass)
	}

	server := newBaseServer(name, namespace, opts)
	server.Spec.Mode = kliov1alpha1.ModeStandard
	server.Spec.Tier1 = &kliov1alpha1.Tier1Configuration{
		Cache: kliov1alpha1.Cache{
			PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
				StorageClassName: sc,
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
		Data: kliov1alpha1.Data{
			PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
				StorageClassName: sc,
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
		EncryptionKeyFile: newFileSource(opts.Encryption.EncryptionKeySecretName, opts.Encryption.EncryptionKeyFileName),
		IdentityFile:      newFileSource(opts.Encryption.IdentitySecretName, opts.Encryption.IdentityFileName),
	}

	// Queue is mandatory when tier1 is configured
	server.Spec.Queue = &kliov1alpha1.Queue{
		PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
			StorageClassName: sc,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
		},
	}

	return server
}

// PluginConfigurationTemplateOptions contains options for creating a PluginConfiguration.
type PluginConfigurationTemplateOptions struct {
	// ServerCertificate is the server certificate for the Klio server.
	ServerCertificate *certmanagerv1.Certificate
	// ClientCertificate is the client certificate for authentication.
	ClientCertificate *certmanagerv1.Certificate
	// ClusterName is the name of the PostgreSQL cluster.
	ClusterName string
	// Mode indicates the operation mode of the plugin.
	Mode kliov1alpha1.ServerMode
	// EnableTier2Backup enables tier2 backup.
	EnableTier2Backup bool
	// EnableTier2Recovery enables tier2 recovery.
	EnableTier2Recovery bool
	// Tier2RetentionPolicy is the retention policy for tier2.
	Tier2RetentionPolicy *kliov1alpha1.RetentionPolicy
}

// GetPluginConfigurationObject returns a Klio PluginConfiguration Object.
func GetPluginConfigurationObject(
	name,
	namespace string,
	opts PluginConfigurationTemplateOptions,
) *kliov1alpha1.PluginConfiguration {
	mode := opts.Mode
	if mode == "" {
		mode = kliov1alpha1.ModeStandard
	}
	spec := kliov1alpha1.PluginConfigurationSpec{
		ServerAddress:    opts.ServerCertificate.Spec.DNSNames[0],
		ClientSecretName: opts.ClientCertificate.Spec.SecretName,
		ServerSecretName: opts.ServerCertificate.Spec.SecretName,
		ClusterName:      opts.ClusterName,
		Mode:             mode,
	}

	// Only populate Tier2 if either backup or recovery is enabled
	if opts.EnableTier2Backup || opts.EnableTier2Recovery {
		spec.Tier2 = &kliov1alpha1.Tier2PluginConfiguration{
			EnableBackup:    opts.EnableTier2Backup,
			EnableRecovery:  opts.EnableTier2Recovery,
			RetentionPolicy: opts.Tier2RetentionPolicy,
		}
	}

	return &kliov1alpha1.PluginConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

// ServerWithTier2TemplateOptions are the options needed to create a Klio server with tier2.
type ServerWithTier2TemplateOptions struct {
	ServerTemplateOptions

	// Tier2Encryption contains the tier2 encryption key and identity file options.
	Tier2Encryption EncryptionOptions
	// S3 contains the S3 configuration parameters for tier2 storage.
	S3 Tier2S3Options
}

// GetServerWithTier2Object returns a Klio server Object with tier1, tier2, and queue configuration.
func GetServerWithTier2Object(
	name,
	namespace string,
	opts ServerWithTier2TemplateOptions,
) *kliov1alpha1.Server {
	// GetServerObject already includes tier1 and queue configuration
	server := GetServerObject(name, namespace, opts.ServerTemplateOptions)

	// Add tier2 configuration
	tier2Config := BuildTier2Configuration(opts.S3, opts.Tier2Encryption, opts.StorageClass)
	server.Spec.Tier2 = &tier2Config

	return server
}

// GetReadOnlyTier2ServerObject returns a read-only Klio server Object with only tier2 configuration.
// This server does not have tier1 or queue, only tier2 for recovery purposes.
func GetReadOnlyTier2ServerObject(
	name,
	namespace string,
	opts ServerWithTier2TemplateOptions,
) *kliov1alpha1.Server {
	server := newBaseServer(name, namespace, opts.ServerTemplateOptions)
	server.Spec.Mode = kliov1alpha1.ModeReadOnly
	tier2Config := BuildTier2Configuration(opts.S3, opts.Tier2Encryption, opts.StorageClass)
	server.Spec.Tier2 = &tier2Config

	return server
}
