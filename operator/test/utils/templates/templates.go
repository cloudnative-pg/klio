package templates

import (
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
)

// KlioServerTemplateOptions are the options needed to create a Klio server.
type KlioServerTemplateOptions struct {
	// TLSSecretName is the secret to be used to expose the Klio server.
	TLSSecretName string

	// ClientCASecretName is the secret that will be used by Kopia and by
	// the Klio WAL server to authenticate users.
	ClientCASecretName string

	// EncryptionSecretName contains the encryption key.
	EncryptionSecretName string
}

// GetKlioServerObject returns a Klio server Object.
func GetKlioServerObject(
	name,
	namespace string,
	opts KlioServerTemplateOptions,
) *kliov1alpha1.Server {
	return &kliov1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kliov1alpha1.ServerSpec{
			Image:              "registry.dev:5000/klio-testing:dev",
			ImagePullPolicy:    corev1.PullAlways,
			TLSSecretName:      opts.TLSSecretName,
			ClientCASecretName: opts.ClientCASecretName,
			CacheConfiguration: kliov1alpha1.CacheConfiguration{
				PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			DataConfiguration: kliov1alpha1.DataConfiguration{
				PersistentVolumeClaimTemplate: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			Password: &cnpgv1.SecretKeySelector{
				LocalObjectReference: api.LocalObjectReference{
					Name: opts.EncryptionSecretName,
				},
				Key: "password",
			},
		},
	}
}

// GetCnpgClusterObject returns a CNPG Cluster Object.
func GetCnpgClusterObject(
	name,
	namespace string,
	instances int,
	pluginConfigurationRef string,
) *cnpgv1.Cluster {
	return &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cnpgv1.ClusterSpec{
			Instances: instances,
			StorageConfiguration: cnpgv1.StorageConfiguration{
				Size: "1Gi",
			},
			LogLevel: "debug",
			PostgresConfiguration: cnpgv1.PostgresConfiguration{
				PgHBA: []string{"local replication all peer"},
			},
			Plugins: []cnpgv1.PluginConfiguration{{
				Name:          "klio.cnpg.io",
				Enabled:       ptr.To(true),
				IsWALArchiver: ptr.To(true),
				Parameters: map[string]string{
					cnpgi.PluginConfigurationRefParam: pluginConfigurationRef,
				},
			}},
		},
	}
}

// GetCnpgClusterWithTablespacesObject returns a CNPG Cluster Object with the passed tablespaces.
func GetCnpgClusterWithTablespacesObject(
	name,
	namespace string,
	instances int,
	pluginConfigurationRef string,
	tablespaces []cnpgv1.TablespaceConfiguration,
) *cnpgv1.Cluster {
	cluster := GetCnpgClusterObject(
		name,
		namespace,
		instances,
		pluginConfigurationRef)

	cluster.Spec.Tablespaces = append(cluster.Spec.Tablespaces, tablespaces...)

	return cluster
}

// GetKlioPluginConfigurationObject returns a Klio PluginConfiguration Object.
func GetKlioPluginConfigurationObject(
	name,
	namespace string,
	serverCertificate *certmanagerv1.Certificate,
	clientCertificate *certmanagerv1.Certificate,
) *kliov1alpha1.PluginConfiguration {
	return &kliov1alpha1.PluginConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kliov1alpha1.PluginConfigurationSpec{
			ServerAddress:    serverCertificate.Spec.DNSNames[0],
			ClientSecretName: clientCertificate.Spec.SecretName,
			ServerSecretName: serverCertificate.Spec.SecretName,
		},
	}
}

// GetCnpgBackupObject returns a CNPG Backup Object.
func GetCnpgBackupObject(
	name,
	namespace string,
	backupTarget cnpgv1.BackupTarget,
	cluster *cnpgv1.Cluster,
) *cnpgv1.Backup {
	return &cnpgv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cnpgv1.BackupSpec{
			Cluster: cnpgv1.LocalObjectReference{
				Name: cluster.Name,
			},
			Target: backupTarget,
			Method: cnpgv1.BackupMethodPlugin,
			PluginConfiguration: &cnpgv1.BackupPluginConfiguration{
				Name: cluster.Spec.Plugins[0].Name,
			},
		},
	}
}
