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

package cnpg

import (
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

// ClusterTemplateOptions configures optional fields for CNPG cluster templates.
type ClusterTemplateOptions struct {
	// StorageClass is the storage class for the cluster's data (and tablespace)
	// PVCs. If empty, the cluster's default storage class is used.
	StorageClass string
}

// GetCnpgClusterObject returns a CNPG Cluster Object.
func GetCnpgClusterObject(
	name,
	namespace string,
	instances int,
	pluginConfigurationRef string,
	opts ClusterTemplateOptions,
) *cnpgv1.Cluster {
	cluster := &cnpgv1.Cluster{
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
				Enabled:       new(true),
				IsWALArchiver: new(true),
				Parameters: map[string]string{
					klioconfig.PluginConfigurationRefParam: pluginConfigurationRef,
				},
			}},
		},
	}

	if opts.StorageClass != "" {
		cluster.Spec.StorageConfiguration.StorageClass = new(opts.StorageClass)
	}

	return cluster
}

// GetCnpgClusterWithTablespacesObject returns a CNPG Cluster Object with the passed tablespaces.
func GetCnpgClusterWithTablespacesObject(
	name,
	namespace string,
	instances int,
	pluginConfigurationRef string,
	tablespaces []cnpgv1.TablespaceConfiguration,
	opts ClusterTemplateOptions,
) *cnpgv1.Cluster {
	cluster := GetCnpgClusterObject(
		name,
		namespace,
		instances,
		pluginConfigurationRef,
		opts)

	cluster.Spec.Tablespaces = append(cluster.Spec.Tablespaces, tablespaces...)

	if opts.StorageClass != "" {
		for i := range cluster.Spec.Tablespaces {
			cluster.Spec.Tablespaces[i].Storage.StorageClass = new(opts.StorageClass)
		}
	}

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
