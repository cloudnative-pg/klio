package cnpgi

import (
	"context"
	"os"

	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/http"
	"github.com/cloudnative-pg/cnpg-i/pkg/backup"
	"github.com/cloudnative-pg/cnpg-i/pkg/metrics"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CNPGI is the implementation of the PostgreSQL sidecar.
type CNPGI struct {
	Client     client.Client
	PluginPath string
}

// Start starts the GRPC service.
func (c *CNPGI) Start(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("cnpgi")
	contextLogger.Info("Starting CNPGI service")

	enrich := func(server *grpc.Server) error {
		backup.RegisterBackupServer(server, BackupServiceImplementation{
			InstanceName: os.Getenv("POD_NAME"),
			Client:       c.Client,
		})
		AddHealthCheck(server)
		metrics.RegisterMetricsServer(server, metricsImpl{})

		return nil
	}

	srv := http.Server{
		IdentityImpl: IdentityImplementation{
			Client: c.Client,
		},
		Enrichers:  []http.ServerEnricher{enrich},
		PluginPath: c.PluginPath,
	}

	return srv.Start(ctx)
}
