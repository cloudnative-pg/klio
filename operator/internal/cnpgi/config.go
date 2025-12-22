package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/pkg/config"
)

const (
	klioArchiveConfigKey = "klio-archive"
	// PluginConfigurationRefParam is the name of the parameter that contains
	// the reference to the Klio PluginConfiguration resource.
	PluginConfigurationRefParam = "pluginConfigurationRef"
)

const (
	// KlioTier1HTTPPort is the default port for Klio Tier1 HTTP API.
	KlioTier1HTTPPort = "51515"
	// KlioTier2HTTPPort is the default port for Klio Tier2 HTTP API.
	KlioTier2HTTPPort = "51516"
	// KlioTier1GRPCPort is the default port for Klio Tier1 gRPC API.
	KlioTier1GRPCPort = "52000"
	// KlioTier2GRPCPort is the default port for Klio Tier2 gRPC API.
	KlioTier2GRPCPort = "52001"
)

func generateKlioConfigForPlugin(
	rawConfiguration *configuration,
	serverCertPath string,
	clientCertPath string,
) (*config.Data, *kliov1alpha1.PluginConfiguration, error) {
	// If the plugin is not configured or not enabled, do nothing
	if rawConfiguration == nil || rawConfiguration.cnpgPluginConfiguration == nil ||
		rawConfiguration.cnpgPluginConfiguration.Name != PluginName ||
		!rawConfiguration.cnpgPluginConfiguration.IsEnabled() {
		return nil, nil, nil
	}

	if rawConfiguration.klioPluginConfiguration == nil {
		return nil, nil, errors.New("missing client parameters configuration for klio plugin")
	}

	klioPluginConfigurationSpec := rawConfiguration.klioPluginConfiguration.Spec

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
				URL:            "https://" + net.JoinHostPort(klioPluginConfigurationSpec.ServerAddress, KlioTier1HTTPPort),
				ServerCertPath: path.Join(serverCertPath, "tls.crt"),
				ClientCertPath: path.Join(clientCertPath, "tls.crt"),
				ClientKeyPath:  path.Join(clientCertPath, "tls.key"),
			},
			Wal: config.WalRepositoryClientConfig{
				Address:        net.JoinHostPort(klioPluginConfigurationSpec.ServerAddress, KlioTier1GRPCPort),
				ClusterName:    klioPluginConfigurationSpec.ClusterName,
				ServerCertPath: path.Join(serverCertPath, "tls.crt"),
				ClientCertPath: path.Join(clientCertPath, "tls.crt"),
				ClientKeyPath:  path.Join(clientCertPath, "tls.key"),
			},
		},
		RetentionPolicy: klioPluginConfigurationSpec.RetentionPolicy,
	}

	if klioPluginConfigurationSpec.Tier2 {
		klioConfig.Client.Base.Tier2URL = "https://" + net.JoinHostPort(
			klioPluginConfigurationSpec.ServerAddress, KlioTier2HTTPPort)
		klioConfig.Client.Wal.Tier2Address = net.JoinHostPort(
			klioPluginConfigurationSpec.ServerAddress, KlioTier2GRPCPort)
	}

	return klioConfig, rawConfiguration.klioPluginConfiguration, nil
}

type configuration struct {
	cnpgPluginConfiguration *cnpgv1.PluginConfiguration
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration
}

func getConfigurations(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
) (map[string]*configuration, error) {
	logger := log.FromContext(ctx).WithName("getConfigurations")

	configurations, err := getArchivePluginConfigurations(ctx, cli, cluster, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to get archive plugin configurations: %w", err)
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
		configurations[serverName] = &configuration{
			cnpgPluginConfiguration: currentPluginConfiguration,
		}

		if ref := currentPluginConfiguration.Parameters[PluginConfigurationRefParam]; ref != "" {
			klioPluginConfiguration := &kliov1alpha1.PluginConfiguration{}
			err := cli.Get(ctx,
				client.ObjectKey{Namespace: cluster.Namespace, Name: ref},
				klioPluginConfiguration)
			if err != nil {
				logger.Error(err, "failed to get client configuration")
				return nil, fmt.Errorf("failed to get '%s' configuration, error: %w", ref, err)
			}
			if hostname := klioPluginConfiguration.Spec.ClusterName; hostname == "" {
				// if the host name is not set, use the server name as the host name
				klioPluginConfiguration.Spec.ClusterName = serverName
			}
			configurations[serverName].klioPluginConfiguration = klioPluginConfiguration
		}
	}

	return configurations, nil
}

func getArchivePluginConfigurations(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
	logger log.Logger,
) (map[string]*configuration, error) {
	configurations := make(map[string]*configuration)

	archivePluginConfiguration := getArchivePluginConfiguration(cluster)
	if archivePluginConfiguration == nil {
		return configurations, nil
	}

	configurations[klioArchiveConfigKey] = &configuration{
		cnpgPluginConfiguration: archivePluginConfiguration,
	}
	ref, err := getParameter(archivePluginConfiguration, PluginConfigurationRefParam)
	if err != nil {
		// we use a more friendly error message for the user
		msg := fmt.Sprintf(
			"'ref' parameter is not specified in the '%s' plugin configuration in the cluster: %s",
			PluginName,
			cluster.Name,
		)
		logger.Error(err, msg)

		return configurations, errors.New(msg)
	}
	if ref == "" {
		msg := fmt.Sprintf(
			"'ref' parameter is empty in the '%s' plugin configuration in the cluster: %s",
			PluginName,
			cluster.Name,
		)
		logger.Error(errors.New("while evaluating ref key content"), msg)

		return configurations, errors.New(msg)
	}

	klioPluginConfiguration := &kliov1alpha1.PluginConfiguration{}
	err = cli.Get(ctx,
		client.ObjectKey{Namespace: cluster.Namespace, Name: ref},
		klioPluginConfiguration)
	if err != nil {
		logger.Error(err, "failed to get client configuration")
		return configurations, fmt.Errorf("failed to get '%s' configuration, error: %w", ref, err)
	}
	if hostname := klioPluginConfiguration.Spec.ClusterName; hostname == "" {
		// if the host name is not set, use the cluster name as the host name
		klioPluginConfiguration.Spec.ClusterName = cluster.Name
	}

	configurations[klioArchiveConfigKey].klioPluginConfiguration = klioPluginConfiguration.DeepCopy()

	return configurations, nil
}

// klioConfigsFromCluster creates configuration data for the main and external plugin usages.
// Returns the map of Klio configs keyed by usage and the parsed cluster plugin configuration, if any.
func klioConfigsFromCluster(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
) (map[string]*config.Data, *kliov1alpha1.PluginConfiguration, error) {
	configs := make(map[string]*config.Data)
	var clusterPC *kliov1alpha1.PluginConfiguration

	configurations, err := getConfigurations(ctx, cli, cluster)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get plugin configurations: %w", err)
	}
	for key, conf := range configurations {
		if conf.cnpgPluginConfiguration == nil || conf.cnpgPluginConfiguration.Name != PluginName ||
			!conf.cnpgPluginConfiguration.IsEnabled() {
			continue
		}
		klioConfig, klioPluginConfig, err := generateKlioConfigForPlugin(
			conf,
			getServerSecretVolumeMountPath(key),
			getClientSecretVolumeMountPath(key),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get plugin configuration: %w", err)
		}
		if klioConfig != nil {
			configs[key] = klioConfig
		}
		if key == klioArchiveConfigKey {
			clusterPC = klioPluginConfig
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

func getParameter(cfg *cnpgv1.PluginConfiguration, name string) (string, error) {
	result, ok := cfg.Parameters[name]
	if !ok || len(result) == 0 {
		return "", &parameterMissingError{name: name}
	}

	return result, nil
}

type parameterMissingError struct {
	name string
}

func (p *parameterMissingError) Error() string {
	return fmt.Sprintf("plugin configuration does not contain %q parameter", p.name)
}
