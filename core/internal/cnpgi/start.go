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
	"context"
	"os"

	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/http"
	"github.com/cloudnative-pg/cnpg-i/pkg/backup"
	"github.com/cloudnative-pg/cnpg-i/pkg/identity"
	restore "github.com/cloudnative-pg/cnpg-i/pkg/restore/job"
	"github.com/cloudnative-pg/cnpg-i/pkg/wal"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CNPGI is the implementation of the PostgreSQL sidecar.
type CNPGI struct {
	Client       client.Client
	PluginPath   string
	enrichers    []http.ServerEnricher
	capabilities []*identity.PluginCapability
}

// AddRestoreCapability adds the restore capability to the CNPGI service.
func (c *CNPGI) AddRestoreCapability(pgDataPath string) {
	enricher := func(server *grpc.Server) error {
		restore.RegisterRestoreJobHooksServer(
			server,
			restoreImpl{PgDataPath: pgDataPath},
		)

		return nil
	}

	c.enrichers = append(c.enrichers, enricher)
	c.capabilities = append(c.capabilities, &identity.PluginCapability{
		Type: &identity.PluginCapability_Service_{
			Service: &identity.PluginCapability_Service{
				Type: identity.PluginCapability_Service_TYPE_RESTORE_JOB,
			},
		},
	})
}

// BackupCapabilityOptions contains options for configuring the backup capability.
type BackupCapabilityOptions struct {
	Tier2 bool
}

// AddBackupCapability adds the backup capability to the CNPGI service.
func (c *CNPGI) AddBackupCapability(opts BackupCapabilityOptions) {
	enricher := func(server *grpc.Server) error {
		podName, ok := os.LookupEnv("POD_NAME")
		if !ok {
			return ErrPodNameNotSet
		}
		backup.RegisterBackupServer(server, backupServiceImplementation{
			InstanceName: podName,
			Tier2:        opts.Tier2,
		})

		return nil
	}

	c.enrichers = append(c.enrichers, enricher)
	c.capabilities = append(c.capabilities, &identity.PluginCapability{
		Type: &identity.PluginCapability_Service_{
			Service: &identity.PluginCapability_Service{
				Type: identity.PluginCapability_Service_TYPE_BACKUP_SERVICE,
			},
		},
	})
}

// WALCapabilityOptions are the options to configure the WAL service.
type WALCapabilityOptions struct {
	Debug bool
}

// AddWALCapability adds the WAL restore capabilities to the CNPGI service.
func (c *CNPGI) AddWALCapability(opts WALCapabilityOptions) {
	enricher := func(server *grpc.Server) error {
		walService := newWalServiceImplementation(newGRPCClientManager(), opts)
		wal.RegisterWALServer(server, &walService)

		return nil
	}

	c.enrichers = append(c.enrichers, enricher)
	c.capabilities = append(c.capabilities, &identity.PluginCapability{
		Type: &identity.PluginCapability_Service_{
			Service: &identity.PluginCapability_Service{
				Type: identity.PluginCapability_Service_TYPE_WAL_SERVICE,
			},
		},
	})
}

// Start starts the GRPC service.
func (c *CNPGI) Start(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("cnpgi")
	contextLogger.Info("Starting CNPGI service")

	healthCheck := func(server *grpc.Server) error {
		addHealthCheck(server)

		return nil
	}

	srv := http.Server{
		IdentityImpl: identityImplementation{capabilities: c.capabilities},
		Enrichers:    append(c.enrichers, healthCheck),
		PluginPath:   c.PluginPath,
	}

	return srv.Start(ctx)
}
