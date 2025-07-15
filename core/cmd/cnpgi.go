// Package cmd is the implementation of the "run" command
package cmd

import (
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
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

	"github.com/cloudnative-pg/klio/core/cmd/clierrors"
	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// cnpgiCmd represents the run command
//
//nolint:gochecknoglobals
var cnpgiCmd = &cobra.Command{
	Use:    "cnpgi",
	Short:  "Start the instance CNPG-i server",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := log.FromContext(cmd.Context())

		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Source == (config.SourceConfig{}) {
			return clierrors.ErrSourceSectionIsRequired
		}

		if configuration.Client == (config.ClientConfig{}) {
			return clierrors.ErrClientSectionIsRequired
		}

		if configuration.Client.Base == (config.BaseRepositoryClientConfig{}) {
			return clierrors.ErrKlioClientSectionIsRequired
		}

		if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
			return clierrors.ErrKlioClientSectionIsRequired
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

		pluginPath, _ := cmd.Flags().GetString("plugin-path")

		if err := mgr.Add(&cnpgi.CNPGI{
			Client:     mgr.GetClient(),
			PluginPath: pluginPath,
		}); err != nil {
			logger.Error(err, "unable to create CNPGI runnable")
			return fmt.Errorf("while creating CNPGI runnable: %w", err)
		}

		return mgr.Start(cmd.Context())
	},
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

//nolint:gochecknoinits
func init() {
	cnpgiCmd.Flags().String(
		"plugin-path",
		"/plugins",
		"The directory where the Unix domain socket should be created",
	)

	rootCmd.AddCommand(cnpgiCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
