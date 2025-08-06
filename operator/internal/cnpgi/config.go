package cnpgi

import (
	"errors"
	"fmt"
	"strconv"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
)

var (
	errPluginNotFound   = errors.New("plugin not found")
	errPluginNotEnabled = errors.New("plugin found but is not enabled")
)

type pluginConfiguration struct {
	ServerAddress    string
	ClusterName      string
	ClientSecretName string
	ServerSecretName string
	BackupName       string
	Enabled          bool
	EnablePPROF      bool
}

func newConfigFromCluster(cluster *cnpgv1.Cluster) (*pluginConfiguration, error) {
	var rawConf *cnpgv1.PluginConfiguration
	for _, plugin := range cluster.Spec.Plugins {
		if plugin.Name != data.GetName() {
			continue
		}

		rawConf = &plugin

		break
	}

	cfg, err := parsePluginConfiguration(rawConf)
	if err != nil {
		return nil, fmt.Errorf("error while parsing plugin configuration: %w", err)
	}
	if cfg.ClusterName == "" {
		cfg.ClusterName = cluster.Name
	}

	return cfg, nil
}

func parsePluginConfiguration(rawConf *cnpgv1.PluginConfiguration) (*pluginConfiguration, error) {
	if rawConf == nil {
		return nil, errPluginNotFound
	}

	if !rawConf.IsEnabled() {
		return nil, errPluginNotEnabled
	}

	var conf pluginConfiguration
	var err error

	conf.ServerAddress, err = getParameter(rawConf, "serverAddress")
	if err != nil {
		return nil, err
	}

	conf.ClientSecretName, err = getParameter(rawConf, "clientSecretName")
	if err != nil {
		return nil, err
	}

	conf.ServerSecretName, err = getParameter(rawConf, "serverSecretName")
	if err != nil {
		return nil, err
	}

	conf.EnablePPROF, err = tryGetBooleanParameter(rawConf, "pprof")
	if err != nil {
		return nil, err
	}

	// not mandatory, so we don't return an error if it's missing
	conf.BackupName, _ = getParameter(rawConf, "backupName")

	// not mandatory, so we don't return an error if it's missing
	conf.ClusterName, _ = getParameter(rawConf, "clusterName")

	return &conf, nil
}

// tryGetParameter attempts to retrieve a parameter from the plugin configuration, returns any error encountered or
// false if the parameter is missing.
func tryGetBooleanParameter(cfg *cnpgv1.PluginConfiguration, name string) (bool, error) {
	result, err := tryGetParameter(cfg, name)
	if err != nil || result == "" {
		return false, err
	}

	return strconv.ParseBool(result)
}

func tryGetParameter(cfg *cnpgv1.PluginConfiguration, name string) (string, error) {
	result, err := getParameter(cfg, name)

	var parameterMissingErr *parameterMissingError
	if errors.As(err, &parameterMissingErr) {
		return "", nil
	}

	return result, err
}

func getParameter(cfg *cnpgv1.PluginConfiguration, name string) (string, error) {
	result, ok := cfg.Parameters[name]
	if !ok || len(result) == 0 {
		return "", &parameterMissingError{
			name: name,
		}
	}

	return result, nil
}

type parameterMissingError struct {
	name string
}

func (p *parameterMissingError) Error() string {
	return fmt.Sprintf("plugin configuration does not contain %q parameter", p.name)
}
