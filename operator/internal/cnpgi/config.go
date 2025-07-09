package cnpgi

import (
	"errors"
	"fmt"
	"strconv"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
)

type pluginConfiguration struct {
	ServerAddress    string
	ClientSecretName string
	ServerSecretName string
	Enabled          bool
	EnablePPROF      bool
}

//nolint:cyclop
func newConfigFromCluster(cluster *cnpgv1.Cluster) (*pluginConfiguration, error) {
	var conf pluginConfiguration
	var rawConf *cnpgv1.PluginConfiguration
	for _, plugin := range cluster.Spec.Plugins {
		if plugin.Name != data.GetName() {
			continue
		}

		rawConf = &plugin

		break
	}

	if rawConf == nil {
		return nil, errors.New("plugin not found")
	}

	if !rawConf.IsEnabled() {
		return nil, errors.New("plugin found but is not enabled")
	}

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

	enablePprofString, err := tryGetParameter(rawConf, "pprof")
	if err != nil {
		return nil, err
	}
	if enablePprofString != "" {
		conf.EnablePPROF, err = strconv.ParseBool(enablePprofString)
		if err != nil {
			return nil, err
		}
	}

	return &conf, nil
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
