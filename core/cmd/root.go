package cmd

import (
	"net/http"
	"os"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/cmd/backup"
	"github.com/cloudnative-pg/klio/core/cmd/server"

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
	Short: "Klio is a Cloud Native Backup & Recovery solution",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
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
func Execute() {
	err := rootCmd.Execute()
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
	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"config",
		"",
		"config file (default is $HOME/.klio.yaml)")
	rootCmd.PersistentFlags().BoolVar(
		&debug,
		"debug",
		false,
		"enable debug logging")
	rootCmd.PersistentFlags().StringVar(
		&pprofServerAddress,
		"pprof-server",
		"",
		"enable the PPROF server using the specified address")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP(
		"toggle",
		"t",
		false,
		"Help message for toggle")

	rootCmd.AddCommand(server.ServerCmd)
	rootCmd.AddCommand(backup.BackupCmd)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
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

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Debug("Using config file", "configFile", viper.ConfigFileUsed())
	}
}
