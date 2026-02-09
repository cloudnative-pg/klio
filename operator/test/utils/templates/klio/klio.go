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

// buildTier2Configuration creates a Tier2Configuration from S3 options and encryption secret.
func buildTier2Configuration(s3Opts Tier2S3Options, encryptionSecretName string) kliov1alpha1.Tier2Configuration {
	return kliov1alpha1.Tier2Configuration{
		Cache: kliov1alpha1.Cache{
			PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
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
		EncryptionKey: &cnpgv1.SecretKeySelector{
			LocalObjectReference: api.LocalObjectReference{
				Name: encryptionSecretName,
			},
			Key: "password",
		},
	}
}

// ServerTemplateOptions are the options needed to create a Klio server.
type ServerTemplateOptions struct {
	// TLSSecretName is the secret to be used to expose the Klio server.
	TLSSecretName string

	// ClientCASecretName is the secret that will be used by Kopia and by
	// the Klio WAL server to authenticate users.
	ClientCASecretName string

	// EncryptionSecretName contains the encryption key.
	EncryptionSecretName string
}

// newBaseServer creates a server with common fields (metadata, image, TLS) but no tier configuration.
func newBaseServer(name, namespace string, opts ServerTemplateOptions) *kliov1alpha1.Server {
	return &kliov1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kliov1alpha1.ServerSpec{
			ImageConfiguration: kliov1alpha1.ImageConfiguration{
				Image:           "registry.dev:5000/klio-testing:dev",
				ImagePullPolicy: corev1.PullAlways,
			},
			TLSConfiguration: kliov1alpha1.TLSConfiguration{
				TLSSecretName:      opts.TLSSecretName,
				ClientCASecretName: opts.ClientCASecretName,
			},
		},
	}
}

// GetServerObject returns a Klio server Object with tier1 configuration.
func GetServerObject(
	name,
	namespace string,
	opts ServerTemplateOptions,
) *kliov1alpha1.Server {
	server := newBaseServer(name, namespace, opts)
	server.Spec.Mode = kliov1alpha1.ModeStandard
	server.Spec.Tier1 = &kliov1alpha1.Tier1Configuration{
		Cache: kliov1alpha1.Cache{
			PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
		Data: kliov1alpha1.Data{
			PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
		EncryptionKey: &cnpgv1.SecretKeySelector{
			LocalObjectReference: api.LocalObjectReference{
				Name: opts.EncryptionSecretName,
			},
			Key: "password",
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
		Mode:             mode,
	}

	// Only populate Tier2 if either backup or recovery is enabled
	if opts.EnableTier2Backup || opts.EnableTier2Recovery {
		spec.Tier2 = kliov1alpha1.Tier2PluginConfiguration{
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

	// Tier2EncryptionSecretName contains the tier2 encryption key.
	Tier2EncryptionSecretName string
	// S3 contains the S3 configuration parameters for tier2 storage.
	S3 Tier2S3Options
}

// GetServerWithTier2Object returns a Klio server Object with tier1, tier2, and queue configuration.
func GetServerWithTier2Object(
	name,
	namespace string,
	opts ServerWithTier2TemplateOptions,
) *kliov1alpha1.Server {
	server := GetServerObject(name, namespace, opts.ServerTemplateOptions)

	// Add tier2 configuration
	tier2Config := buildTier2Configuration(opts.S3, opts.Tier2EncryptionSecretName)
	server.Spec.Tier2 = &tier2Config

	// Add queue configuration (mandatory when tier1 and tier2 are both configured)
	server.Spec.Queue = &kliov1alpha1.Queue{
		PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
		},
	}

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
	tier2Config := buildTier2Configuration(opts.S3, opts.Tier2EncryptionSecretName)
	server.Spec.Tier2 = &tier2Config

	return server
}
