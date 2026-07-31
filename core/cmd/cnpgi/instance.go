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

package cnpgi

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// instanceCmd represents the `klio cnpgi instance` command
//
//nolint:gochecknoglobals
var instanceCmd = &cobra.Command{
	Use:    "instance",
	Short:  "Start the instance CNPG-I server",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		shutdownOtel := opentelemetry.Init(cmd.Context())
		defer shutdownOtel()

		archiveConfigFile, _ := cmd.Root().PersistentFlags().GetString("config")
		pluginPath, _ := cmd.Flags().GetString("plugin-path")

		var configuration config.Data
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		debug, _ := cmd.PersistentFlags().GetBool("debug")
		podName, _ := cmd.Flags().GetString("pod-name")
		clusterName, _ := cmd.Flags().GetString("cluster-name")
		clusterNamespace, _ := cmd.Flags().GetString("cluster-namespace")

		capabilities := func(server *cnpgi.CNPGI) {
			server.AddBackupCapability(cnpgi.BackupCapabilityOptions{
				Tier2: configuration.Tier2BackupEnabled,
			})
			server.AddWALCapability(cnpgi.WALCapabilityOptions{
				Debug: debug,
			})
		}

		enrichManager := func(mgr manager.Manager) error {
			if archiveConfigFile != "" {
				sendWal := cnpgi.SendWalClusterReconciler{
					Client:         mgr.GetClient(),
					PodName:        podName,
					KlioConfigFile: archiveConfigFile,
				}

				return sendWal.SetupWithManager(mgr)
			}

			return nil
		}

		return runCNPGI(
			cmd.Context(),
			pluginPath,
			archiveConfigFile,
			types.NamespacedName{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
			capabilities,
			enrichManager,
		)
	},
}

//nolint:gochecknoinits
func init() {
	instanceCmd.Flags().String(
		"plugin-path",
		"/plugins",
		"The directory where the Unix domain socket should be created",
	)
	instanceCmd.Flags().String(
		"cluster-name",
		"",
		"The name of the cluster object",
	)
	instanceCmd.Flags().String(
		"cluster-namespace",
		"",
		"The namespace of the cluster object",
	)
	instanceCmd.Flags().String(
		"pod-name",
		"",
		"The name of the current instance",
	)

	CnpgiCmd.AddCommand(instanceCmd)
}
