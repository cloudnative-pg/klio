package cnpgi

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func runCNPGI(ctx context.Context, pluginPath string, addCapabilities func(server *cnpgi.CNPGI)) error {
	logger := log.FromContext(ctx)

	var configuration config.Data

	// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
	// when using environment variables
	if err := viper.Unmarshal(&configuration); err != nil {
		return fmt.Errorf("could not unmarshal configuration: %w", err)
	}

	// Sets the defaults values, to be overridden by the user configuration
	configuration.SetDefaults()

	if configuration.Source == (config.SourceConfig{}) {
		return cli.ErrSourceSectionIsRequired
	}

	if configuration.Client == (config.ClientConfig{}) {
		return cli.ErrClientSectionIsRequired
	}

	if configuration.Client.Base == (config.BaseRepositoryClientConfig{}) {
		return cli.ErrKlioClientSectionIsRequired
	}

	if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
		return cli.ErrKlioClientSectionIsRequired
	}

	if errs := validator.Validate(&configuration); errs != nil {
		return fmt.Errorf("configuration validation error: %w", errs)
	}

	controllerOptions := ctrl.Options{
		Scheme: generateScheme(),
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.Secret{},
					&cnpgv1.Cluster{},
					&cnpgv1.Backup{},
				},
			},
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

	return mgr.Start(ctx)
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
