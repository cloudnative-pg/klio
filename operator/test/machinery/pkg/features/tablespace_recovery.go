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

package features

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/postgres"
)

// TablespaceRecoveryFeature defines a feature for testing recovery of a backup in the CloudNativePG operator.
type TablespaceRecoveryFeature struct {
	*RecoveryFeature

	sourceTablespaceConfig *[]cnpgv1.TablespaceConfiguration
}

// TablespaceRecoveryFeatureConfig holds the configuration for creating a recovery feature test.
type TablespaceRecoveryFeatureConfig struct {
	*RecoveryFeatureConfig

	// SourceTablespaceConfig is the tablespaces list created in the source Cluster.
	SourceTablespaceConfig *[]cnpgv1.TablespaceConfiguration
}

// NewTablespaceRecoveryFeature creates a new TablespaceRecoveryFeature with the given
// configuration and default timeouts.
func NewTablespaceRecoveryFeature(
	config TablespaceRecoveryFeatureConfig,
) *TablespaceRecoveryFeature {
	return &TablespaceRecoveryFeature{
		RecoveryFeature:        NewRecoveryFeature(*config.RecoveryFeatureConfig),
		sourceTablespaceConfig: config.SourceTablespaceConfig,
	}
}

// Name returns the name of the recovery feature.
func (f *TablespaceRecoveryFeature) Name() string {
	return f.name
}

// Setup initializes the recovery feature test, setting up the necessary resources.
func (f *TablespaceRecoveryFeature) Setup() types.StepFunc {
	return f.setup
}

// Run executes the recovery feature test, creating a Cluster from a backup and waiting for it to complete.
func (f *TablespaceRecoveryFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running tablespace recovery feature test")
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Create table in tablespaces in source cluster database
		for _, tablespace := range *f.sourceTablespaceConfig {
			tablespaceQuery := fmt.Sprintf(
				"CREATE TABLE numbers_%s TABLESPACE %s AS SELECT generate_series(1, 1000) AS x;",
				tablespace.Name,
				tablespace.Name)

			// Insert some test data in the source Cluster
			_, err = postgres.ExecPostgresQuery(ctx, r, f.sourcePrimaryPod, "postgres", tablespaceQuery)
			require.NoError(t, err, "failed to create table in tablespace %s", tablespace.Name)
			require.NoError(t, postgres.CheckpointAndSwitchWal(ctx, r, f.sourcePrimaryPod),
				"failed to checkpoint and switch WAL")
		}

		// Take a backup
		require.NoError(t, r.Create(ctx, f.backup), "failed to create backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, f.backup),
			wait.WithTimeout(f.backupTimeout),
			wait.WithInterval(f.backupCheckInterval),
		)
		require.NoError(t, err, "backup not completed")

		// Mutate recovery cluster
		for _, mutate := range f.mutateRecoveryCluster {
			require.NoError(t, mutate(ctx, f.recoveryCluster, r), "failed to mutate recovery cluster")
		}

		// Recovery
		require.NoError(t, r.Create(ctx, f.recoveryCluster), "failed to create a recovery cluster")
		err = wait.For(
			machineryConditions.ClusterIsReady(r, f.recoveryCluster),
			wait.WithTimeout(f.recoveryTimeout),
			wait.WithInterval(f.recoveryCheckInterval),
		)
		require.NoError(t, err, "recovery Cluster not ready")

		// Verify tablespaces in the recovered Cluster
		var recoveryPrimaryPod corev1.Pod
		require.NoError(t,
			r.Get(ctx, f.recoveryCluster.Status.CurrentPrimary, f.recoveryCluster.Namespace, &recoveryPrimaryPod),
			"failed to get the recovery Cluster primary pod")

		for _, tablespace := range *f.sourceTablespaceConfig {
			// Get the tablespace name and owner from postgres catalog
			tablespaceQuery := fmt.Sprintf(
				"SELECT t.spcname, a.rolname "+
					"FROM pg_catalog.pg_tablespace AS t "+
					"JOIN pg_catalog.pg_authid AS a ON t.spcowner = a.oid "+
					"JOIN pg_catalog.pg_class AS c ON t.oid = c.reltablespace "+
					"WHERE c.relname = 'numbers_%s';",
				tablespace.Name)
			out, err := postgres.ExecPostgresQuery(ctx, r, &recoveryPrimaryPod, "postgres",
				tablespaceQuery)
			require.NoError(t, err, "failed to get tablespace from database for tablespace %s", tablespace.Name)
			tablespaceSlice := strings.Split(out, "|")
			require.Len(t, tablespaceSlice, 2, "expected 2 fields from tablespace query for tablespace %s",
				tablespace.Name)
			tablespaceName := strings.TrimSpace(tablespaceSlice[0])
			tablespaceOwner := strings.TrimSpace(tablespaceSlice[1])

			// Compare the configured tablespace name in the original cluster with the one existing in the recovered one
			require.Equal(t, tablespace.Name, tablespaceName)
			// Compare the configured tablespace owner in the original cluster with the one existing in the recovered one
			require.Equal(t, tablespace.Owner.Name, tablespaceOwner)

			// Verify data in the recovered table
			verifyQuery := fmt.Sprintf("SELECT COUNT(*) FROM numbers_%s;", tablespaceName)
			out, err = postgres.ExecPostgresQuery(ctx, r, &recoveryPrimaryPod, "postgres", verifyQuery)
			require.NoError(t, err, "failed to verify data in tablespace %s", tablespaceName)
			countedEntries, err := strconv.Atoi(strings.TrimSpace(out))
			require.NoError(t, err, "failed to parse row count for tablespace %s", tablespaceName)
			require.Equal(t, 1000, countedEntries, "expected 1000 rows in tablespace %s", tablespaceName)
		}

		return ctx
	}
}

// Teardown cleans up resources after the test is run.
func (f *TablespaceRecoveryFeature) Teardown() types.StepFunc {
	return f.teardown
}
