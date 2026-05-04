package cnpg

import (
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

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
				PgHBA: []string{"local replication all peer map=local"},
			},
			Plugins: []cnpgv1.PluginConfiguration{{
				Name:          "klio.cnpg.io",
				Enabled:       ptr.To(true),
				IsWALArchiver: ptr.To(true),
				Parameters: map[string]string{
					klioconfig.PluginConfigurationRefParam: pluginConfigurationRef,
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
