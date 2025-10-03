package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path"
	"strconv"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/cloudnative-pg/klio/operator/pkg/config"
)

var (
	errPluginNotFound   = errors.New("plugin not found")
	errPluginNotEnabled = errors.New("plugin found but is not enabled")
)

const klioArchiveConfigKey = "klio-archive"

type pluginConfiguration struct {
	ServerAddress          string
	ClusterName            string
	ClientSecretName       string
	ServerSecretName       string
	BackupRef              string
	BackupID               string
	MetricsAddressInstance string
	MetricsAddressRestore  string
	MetricsAddressSendWal  string
	Enabled                bool
	EnablePPROF            bool
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
	conf.BackupRef, _ = getParameter(rawConf, "backupRef")

	// not mandatory, so we don't return an error if it's missing
	conf.BackupID, _ = getParameter(rawConf, "backupID")

	// not mandatory, so we don't return an error if it's missing
	conf.ClusterName, _ = getParameter(rawConf, "clusterName")

	// not mandatory, so we don't return an error if it's missing
	conf.MetricsAddressInstance, _ = getParameter(rawConf, "metricsAddressInstance")

	// not mandatory, so we don't return an error if it's missing
	conf.MetricsAddressSendWal, _ = getParameter(rawConf, "metricsAddressSendWal")

	// not mandatory, so we don't return an error if it's missing
	conf.MetricsAddressRestore, _ = getParameter(rawConf, "metricsAddressRestore")

	return &conf, nil
}

func generateKlioConfigForPlugin(
	ctx context.Context,
	cli client.Client,
	rawConfiguration *cnpgv1.PluginConfiguration,
	certPath string,
	namespace string,
) (*config.Data, *pluginConfiguration, error) {
	// If the plugin is not configured or not enabled, do nothing
	if rawConfiguration == nil ||
		rawConfiguration.Name != PluginName ||
		!rawConfiguration.IsEnabled() {
		return nil, nil, nil
	}

	configuration, err := parsePluginConfiguration(rawConfiguration)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse plugin configuration: %w", err)
	}

	clientSecret := &corev1.Secret{}
	err = cli.Get(ctx,
		client.ObjectKey{Namespace: namespace, Name: configuration.ClientSecretName},
		clientSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get client secret %q: %w", configuration.ClientSecretName, err)
	}

	// Extract the data from the client secret
	username := string(clientSecret.Data[corev1.BasicAuthUsernameKey])
	password := string(clientSecret.Data[corev1.BasicAuthPasswordKey])

	klioConfig := &config.Data{
		Source: config.SourceConfig{
			DSN:         "user=postgres replication=yes application_name=klio",
			StandardDSN: "user=postgres application_name=klio",
			Slot:        "klio",
			// The following parameters are not used by the plugin, but here with their default for completeness
			StandbyMessageTimeoutSeconds: 0,
			FlushTimeoutMilliseconds:     0,
			BufferSize:                   0,
		},
		Client: config.ClientConfig{
			Base: config.BaseRepositoryClientConfig{
				URL:            "https://" + net.JoinHostPort(configuration.ServerAddress, "51515"),
				ServerCertPath: path.Join(certPath, "tls.crt"),
				Hostname:       configuration.ClusterName,
				Username:       username,
				Password:       password,
			},
			Wal: config.WalRepositoryClientConfig{
				Address:        net.JoinHostPort(configuration.ServerAddress, "52000"),
				ClusterName:    configuration.ClusterName,
				ServerCertPath: path.Join(certPath, "tls.crt"),
				Username:       username,
				Password:       password,
			},
		},
	}

	return klioConfig, configuration, nil
}

func getPluginConfigurations(cluster *cnpgv1.Cluster) map[string]*cnpgv1.PluginConfiguration {
	pluginConfigurations := make(map[string]*cnpgv1.PluginConfiguration)

	archivePluginConfiguration := getArchivePluginConfiguration(cluster)
	if archivePluginConfiguration != nil {
		hostname, err := getParameter(archivePluginConfiguration, "clusterName")
		if err != nil || hostname == "" {
			// if the hostname is not set, use the cluster name as the hostname
			archivePluginConfiguration.Parameters["clusterName"] = cluster.Name
		}
		pluginConfigurations[klioArchiveConfigKey] = archivePluginConfiguration
	}

	for _, externalCluster := range cluster.Spec.ExternalClusters {
		if externalCluster.PluginConfiguration == nil {
			continue
		}

		if externalCluster.PluginConfiguration.Name != PluginName {
			continue
		}

		if !externalCluster.PluginConfiguration.IsEnabled() {
			continue
		}

		serverName := externalCluster.GetServerName()
		currentPluginConfiguration := externalCluster.PluginConfiguration.DeepCopy()
		pluginConfigurations[serverName] = currentPluginConfiguration

		if _, ok := currentPluginConfiguration.Parameters["clusterName"]; !ok {
			// if the hostname is not set, use the server name as the cluster name
			pluginConfigurations[serverName].Parameters["clusterName"] = serverName
		}
	}

	return pluginConfigurations
}

// klioConfigsFromCluster creates configuration data for the main and external plugin usages.
// Returns the map of Klio configs keyed by usage and the parsed cluster plugin configuration, if any.
func klioConfigsFromCluster(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
) (map[string]*config.Data, *pluginConfiguration, error) {
	configs := make(map[string]*config.Data)
	var clusterPC *pluginConfiguration

	rawPluginConfiguration := getPluginConfigurations(cluster)
	for key, pc := range rawPluginConfiguration {
		if pc == nil || pc.Name != PluginName || !pc.IsEnabled() {
			continue
		}
		klioConfig, parsedPC, err := generateKlioConfigForPlugin(
			ctx,
			cli,
			pc,
			getServerSecretVolumeMountPath(key),
			cluster.Namespace,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get plugin configuration: %w", err)
		}
		if klioConfig != nil {
			configs[key] = klioConfig
		}
		if key == klioArchiveConfigKey {
			clusterPC = parsedPC
		}
	}

	return configs, clusterPC, nil
}

func klioConfigsToSecret(
	configs map[string]*config.Data,
	cluster *cnpgv1.Cluster,
) (*corev1.Secret, error) {
	secretStringData := make(map[string][]byte)

	for k, v := range configs {
		yamlData, err := yaml.Marshal(&v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal klio config to yaml: %w", err)
		}
		secretStringData[k] = yamlData
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + klioConfigSecretNameSuffix,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: cluster.APIVersion,
					Kind:       cluster.Kind,
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
		},
		Data: secretStringData,
	}

	return secret, nil
}

func getArchivePluginConfiguration(cluster *cnpgv1.Cluster) *cnpgv1.PluginConfiguration {
	var rawConf *cnpgv1.PluginConfiguration
	for _, plugin := range cluster.Spec.Plugins {
		if plugin.Name != PluginName {
			continue
		}

		rawConf = &plugin

		break
	}

	return rawConf
}

// tryGetBooleanParameter attempts to retrieve a parameter from the plugin configuration, returns any error
// encountered or false if the parameter is missing.
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
