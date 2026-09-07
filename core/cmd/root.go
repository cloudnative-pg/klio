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

package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/cmd/admin"
	"github.com/cloudnative-pg/klio/core/cmd/backup"
	"github.com/cloudnative-pg/klio/core/cmd/cnpgi"
	"github.com/cloudnative-pg/klio/core/cmd/retention"
	"github.com/cloudnative-pg/klio/core/cmd/server"
	"github.com/cloudnative-pg/klio/core/cmd/walplayer"

	_ "net/http/pprof" //nolint:gosec
)

//nolint:gochecknoglobals
var cfgFile string

//nolint:gochecknoglobals
var debug bool

//nolint:gochecknoglobals
var logFlags = &log.Flags{}

//nolint:gochecknoglobals
var pprofServerAddress string

// rootCmd represents the base command when called without any subcommands
//
//nolint:gochecknoglobals
var rootCmd = &cobra.Command{
	Use:   "klio",
	Short: "PostgreSQL Backup & Recovery for CloudNativePG",
	Long: `Klio is a backup and recovery engine for PostgreSQL clusters managed by CloudNativePG.

This CLI is primarily invoked internally by the Klio Operator: it runs inside
Klio Server pods and as sidecar containers in the PostgreSQL instance pods
managed by CloudNativePG. Most of its commands are not generally meant to be
run directly by end users.`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Silence usage for runtime errors.
		// Usage is still shown for flag parsing and args validation errors
		// because those occur before PersistentPreRunE is called.
		cmd.SilenceUsage = true

		// TODO: fix, it is for backward compatibility
		if debug {
			log.SetLogLevel(log.DebugLevelString)
		}
		logFlags.ConfigureLogging()

		if pprofServerAddress != "" {
			go func() {
				log.Info("Starting PPROF server", "pprofServerAddress", pprofServerAddress)
				err := http.ListenAndServe(pprofServerAddress, nil) //nolint:gosec
				if err != nil {
					log.Error(err, "Error while starting the PPROF server")
				}
			}()
		}

		return nil
	},
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(ctx context.Context) {
	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}

//nolint:gochecknoinits
func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	logFlags.AddFlags(rootCmd.PersistentFlags())
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.klio.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVar(&pprofServerAddress, "pprof-server", "",
		"enable the PPROF server using the specified address")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.AddCommand(server.ServerCmd)
	rootCmd.AddCommand(backup.BackupCmd)
	rootCmd.AddCommand(walplayer.WalPlayerCmd)
	rootCmd.AddCommand(cnpgi.CnpgiCmd)
	rootCmd.AddCommand(retention.RetentionCmd)
	rootCmd.AddCommand(admin.AdminCmd)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigType("yaml")
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".klio" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".klio")
	}

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Bind AWS_REGION as a fallback for the S3 region configuration.
	// This allows the AWS SDK's standard environment variable to be used.
	_ = viper.BindEnv("tier2.s3.region", "TIER2_S3_REGION", "AWS_REGION")

	// Bind CUSTOM_CNPG_GROUP and CUSTOM_CNPG_VERSION for the sidecar.
	// The operator sets these env vars with underscores, but viper keys use dashes.
	_ = viper.BindEnv("custom-cnpg-group", "CUSTOM_CNPG_GROUP")
	_ = viper.BindEnv("custom-cnpg-version", "CUSTOM_CNPG_VERSION")

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		if err, ok := errors.AsType[viper.ConfigFileNotFoundError](err); ok {
			log.Debug("No config file found")
		} else {
			log.Error(err, "Failed reading config file")
		}
	}
}
