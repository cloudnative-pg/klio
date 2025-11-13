package cnpgi

import (
	"context"
	"os"

	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/http"
	"github.com/cloudnative-pg/cnpg-i/pkg/backup"
	"github.com/cloudnative-pg/cnpg-i/pkg/identity"
	"github.com/cloudnative-pg/cnpg-i/pkg/metrics"
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

// AddBackupCapability adds the backup capability to the CNPGI service.
func (c *CNPGI) AddBackupCapability() {
	enricher := func(server *grpc.Server) error {
		backup.RegisterBackupServer(server, backupServiceImplementation{
			InstanceName: os.Getenv("POD_NAME"),
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

// AddWALCapability adds the WAL restore capabilities to the CNPGI service.
func (c *CNPGI) AddWALCapability(enableDebug bool) {
	enricher := func(server *grpc.Server) error {
		walService := walServiceImplementation{
			enableDebug: enableDebug,
		}
		wal.RegisterWALServer(server, walService)

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

// AddMetricsCapability adds the backup capability to the CNPGI service.
func (c *CNPGI) AddMetricsCapability() {
	enricher := func(server *grpc.Server) error {
		metrics.RegisterMetricsServer(server, metricsImpl{})

		return nil
	}

	c.enrichers = append(c.enrichers, enricher)
	c.capabilities = append(c.capabilities, &identity.PluginCapability{
		Type: &identity.PluginCapability_Service_{
			Service: &identity.PluginCapability_Service{
				Type: identity.PluginCapability_Service_TYPE_METRICS,
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
