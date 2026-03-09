package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
)

//nolint:cyclop
func runCNPGI(
	ctx context.Context,
	pluginPath string,
	configFile string,
	clusterKey types.NamespacedName,
	addCapabilities func(server *cnpgi.CNPGI),
	enrichManager func(m manager.Manager) error,
) error {
	logger := log.FromContext(ctx)

	// Create a context that gets cancelled on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	controllerOptions := ctrl.Options{
		Scheme: generateScheme(),
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&cnpgv1.Cluster{}: {
					Field: fields.OneTermEqualSelector("metadata.name", clusterKey.Name),
					Namespaces: map[string]cache.Config{
						clusterKey.Namespace: {},
					},
				},
			},
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.Secret{},
					&cnpgv1.Cluster{},
					&cnpgv1.Backup{},
				},
			},
		},
		Metrics: server.Options{
			// Disable the metrics endpoint since we're using OTEL bridge
			BindAddress: "0",
		},
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), controllerOptions)
	if err != nil {
		logger.Error(err, "unable to start manager")
		return err
	}

	server := &cnpgi.CNPGI{
		Client:     mgr.GetClient(),
		PluginPath: pluginPath,
	}
	addCapabilities(server)

	if err := mgr.Add(server); err != nil {
		logger.Error(err, "unable to create CNPGI runnable")
		return fmt.Errorf("while creating CNPGI runnable: %w", err)
	}

	// Enrich the manager
	if enrichManager != nil {
		if err := enrichManager(mgr); err != nil {
			return err
		}
	}

	// Add config file watcher so the sidecar restarts when the config changes
	if configFile != "" {
		if err := mgr.Add(
			cnpgi.NewConfigFileWatcher(configFile, 10*time.Second),
		); err != nil {
			return fmt.Errorf("while adding config watcher: %w", err)
		}
	}

	// Start the manager and handle graceful shutdown
	if err := mgr.Start(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logger.Info("Manager stopped due to context cancellation, exiting gracefully")
			return nil
		}

		if errors.Is(err, cnpgi.ErrConfigFileChanged) {
			logger.Info("Manager stopped due to configuration file change, exiting gracefully")
			return nil
		}

		return err
	}

	return nil
}

// generateScheme creates a runtime.Scheme object with all the
// definition needed to support the sidecar. This allows
// the plugin to be used in every CNPG-based operator.
func generateScheme() *runtime.Scheme {
	result := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(result))

	cnpgGroup := viper.GetString("custom-cnpg-group")
	cnpgVersion := viper.GetString("custom-cnpg-version")
	if len(cnpgGroup) == 0 {
		cnpgGroup = cnpgv1.SchemeGroupVersion.Group
	}
	if len(cnpgVersion) == 0 {
		cnpgVersion = cnpgv1.SchemeGroupVersion.Version
	}

	// Proceed with custom registration of the CNPG scheme
	schemeGroupVersion := schema.GroupVersion{Group: cnpgGroup, Version: cnpgVersion}
	schemeBuilder := &scheme.Builder{GroupVersion: schemeGroupVersion}
	schemeBuilder.Register(&cnpgv1.Cluster{}, &cnpgv1.ClusterList{})
	schemeBuilder.Register(&cnpgv1.Backup{}, &cnpgv1.BackupList{})
	schemeBuilder.Register(&cnpgv1.ScheduledBackup{}, &cnpgv1.ScheduledBackupList{})
	utilruntime.Must(schemeBuilder.AddToScheme(result))

	log.Info("CNPG types registration", "schemeGroupVersion", schemeGroupVersion)

	return result
}
