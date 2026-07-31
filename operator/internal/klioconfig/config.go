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

// Package klioconfig provides shared configuration logic for building
// and managing Klio config data across the operator and reconcilers.
package klioconfig

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/pkg/config"
)

const (
	// PluginName is the name of the Klio plugin.
	PluginName = "klio.cnpg.io"

	// ArchiveConfigKey is the key used for the archive plugin configuration.
	ArchiveConfigKey = "klio-archive"

	// PluginConfigurationRefParam is the name of the parameter that contains
	// the reference to the Klio PluginConfiguration resource.
	PluginConfigurationRefParam = "pluginConfigurationRef"

	// tlsCertFile is the standard TLS certificate filename.
	tlsCertFile = "tls.crt"
	// tlsKeyFile is the standard TLS private key filename.
	tlsKeyFile = "tls.key"
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

// ClusterPlugins maps config keys to their resolved PluginConfiguration.
// All entries are validated and filtered — only enabled Klio plugins
// with a valid PluginConfiguration are included.
type ClusterPlugins map[string]*kliov1alpha1.PluginConfiguration

// ArchivePluginConfiguration returns the PluginConfiguration for the
// archive key, or nil.
func (cp ClusterPlugins) ArchivePluginConfiguration() *kliov1alpha1.PluginConfiguration {
	return cp[ArchiveConfigKey]
}

// TypeLabelKey is the label key used to identify klio-config secrets.
const TypeLabelKey = "klio.cnpg.io/type"

// TypeLabelValue is the label value used to identify klio-config secrets.
const TypeLabelValue = "klio-config"

// ConfigDataKey is the key used for the config data in klio-config secrets.
const ConfigDataKey = "config.yaml"

// GenerateConfig builds a config.Data from a PluginConfigurationSpec.
// configKey is the configuration key (e.g. "klio-archive").
// clusterName is the default cluster name when the PC doesn't set one.
func GenerateConfig(
	spec kliov1alpha1.PluginConfigurationSpec,
	configKey string,
) *config.Data {
	serverCertPath := GetServerSecretVolumeMountPath(configKey)
	clientCertPath := GetClientSecretVolumeMountPath(configKey)

	klioTier1RetentionPolicy := convertTier1RetentionPolicy(spec.Tier1)
	klioTier2RetentionPolicy := convertTier2RetentionPolicy(spec.Tier2)
	walPrefetch := spec.GetWALPrefetch()

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
			ClusterName: spec.ClusterName,
			Base: config.BaseRepositoryClientConfig{
				ServerCertPath: path.Join(serverCertPath, tlsCertFile),
				ClientCertPath: path.Join(clientCertPath, tlsCertFile),
				ClientKeyPath:  path.Join(clientCertPath, tlsKeyFile),
			},
			Wal: config.WalRepositoryClientConfig{
				ServerCertPath: path.Join(serverCertPath, tlsCertFile),
				ClientCertPath: path.Join(clientCertPath, tlsCertFile),
				ClientKeyPath:  path.Join(clientCertPath, tlsKeyFile),
			},
		},
		Tier1RetentionPolicy: klioTier1RetentionPolicy,
		Tier2RetentionPolicy: klioTier2RetentionPolicy,
		WALPrefetch: config.WALPrefetchConfig{
			Count:                  walPrefetch.Count,
			MaxConcurrentDownloads: walPrefetch.MaxConcurrentDownloads,
		},
		Tier1Enabled: spec.Mode != kliov1alpha1.ModeReadOnly,
	}

	if spec.Tier2 != nil {
		klioConfig.Tier2BackupEnabled = spec.Tier2.EnableBackup
		klioConfig.Tier2RecoveryEnabled = spec.Tier2.EnableRecovery
	}

	if spec.Mode != kliov1alpha1.ModeReadOnly {
		klioConfig.Client.Base.URL = "https://" + net.JoinHostPort(
			spec.ServerAddress, KlioTier1HTTPPort)
		klioConfig.Client.Wal.Address = net.JoinHostPort(
			spec.ServerAddress, KlioTier1GRPCPort)
	}

	if spec.Tier2 != nil && (spec.Tier2.EnableBackup || spec.Tier2.EnableRecovery) {
		klioConfig.Client.Base.Tier2URL = "https://" + net.JoinHostPort(
			spec.ServerAddress, KlioTier2HTTPPort)
		klioConfig.Client.Wal.Tier2Address = net.JoinHostPort(
			spec.ServerAddress, KlioTier2GRPCPort)
	}

	return klioConfig
}

func convertRetentionPolicy(p *kliov1alpha1.RetentionPolicy) *config.RetentionPolicy {
	if p == nil {
		return nil
	}

	return &config.RetentionPolicy{
		KeepLatest:  p.KeepLatest,
		KeepAnnual:  p.KeepAnnual,
		KeepMonthly: p.KeepMonthly,
		KeepWeekly:  p.KeepWeekly,
		KeepDaily:   p.KeepDaily,
		KeepHourly:  p.KeepHourly,
	}
}

func convertTier1RetentionPolicy(tier1 *kliov1alpha1.Tier1PluginConfiguration) *config.RetentionPolicy {
	if tier1 == nil {
		return nil
	}

	return convertRetentionPolicy(tier1.RetentionPolicy)
}

func convertTier2RetentionPolicy(tier2 *kliov1alpha1.Tier2PluginConfiguration) *config.RetentionPolicy {
	if tier2 == nil {
		return nil
	}

	return convertRetentionPolicy(tier2.RetentionPolicy)
}

// configuration holds the CNPG and Klio plugin configurations for a single config key.
// It is used internally by getConfigurations and is not exported.
type configuration struct {
	cnpgPluginConfiguration *cnpgv1.PluginConfiguration
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration
}

// getConfigurations retrieves all plugin configurations for a cluster.
func getConfigurations(
	// NOSONAR
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
		if err := addExternalClusterConfiguration(ctx, cli, cluster, externalCluster, configurations, logger); err != nil {
			return nil, err
		}
	}

	return configurations, nil
}

func addExternalClusterConfiguration(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
	externalCluster cnpgv1.ExternalCluster,
	configurations map[string]*configuration,
	logger log.Logger,
) error {
	if externalCluster.PluginConfiguration == nil ||
		externalCluster.PluginConfiguration.Name != PluginName ||
		!externalCluster.PluginConfiguration.IsEnabled() {
		return nil
	}

	serverName := externalCluster.GetServerName()
	currentPluginConfiguration := externalCluster.PluginConfiguration.DeepCopy()
	configurations[serverName] = &configuration{
		cnpgPluginConfiguration: currentPluginConfiguration,
	}

	ref := currentPluginConfiguration.Parameters[PluginConfigurationRefParam]
	if ref == "" {
		return nil
	}

	klioPluginConfiguration := &kliov1alpha1.PluginConfiguration{}
	if err := cli.Get(ctx,
		client.ObjectKey{Namespace: cluster.Namespace, Name: ref},
		klioPluginConfiguration); err != nil {
		if apierrors.IsNotFound(err) {
			return &PluginConfigurationNotFoundError{Name: ref}
		}

		logger.Error(err, "failed to get client configuration")

		return fmt.Errorf("failed to get '%s' configuration, error: %w", ref, err)
	}

	if klioPluginConfiguration.Spec.ClusterName == "" {
		klioPluginConfiguration.Spec.ClusterName = serverName
	}
	configurations[serverName].klioPluginConfiguration = klioPluginConfiguration

	return nil
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

	configurations[ArchiveConfigKey] = &configuration{
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
		if apierrors.IsNotFound(err) {
			return configurations, &PluginConfigurationNotFoundError{Name: ref}
		}

		logger.Error(err, "failed to get client configuration")

		return configurations, fmt.Errorf("failed to get '%s' configuration, error: %w", ref, err)
	}
	if klioPluginConfiguration.Spec.ClusterName == "" {
		// if the host name is not set, use the cluster name as the host name
		klioPluginConfiguration.Spec.ClusterName = cluster.Name
	}

	configurations[ArchiveConfigKey].klioPluginConfiguration = klioPluginConfiguration.DeepCopy()

	return configurations, nil
}

// ResolveClusterPlugins resolves all plugin configurations for a cluster,
// filtering out disabled or invalid entries, and returns the fully resolved
// plugin data ready for downstream consumption.
func ResolveClusterPlugins(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
) (ClusterPlugins, error) {
	configurations, err := getConfigurations(ctx, cli, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin configurations: %w", err)
	}

	plugins := make(ClusterPlugins)
	for key, conf := range configurations {
		if conf.cnpgPluginConfiguration == nil ||
			conf.cnpgPluginConfiguration.Name != PluginName ||
			!conf.cnpgPluginConfiguration.IsEnabled() {
			continue
		}

		if conf.klioPluginConfiguration == nil {
			continue
		}

		plugins[key] = conf.klioPluginConfiguration
	}

	return plugins, nil
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

// PluginConfigurationNotFoundError is returned when a PluginConfiguration does not exist.
type PluginConfigurationNotFoundError struct {
	Name string
}

func (e *PluginConfigurationNotFoundError) Error() string {
	return fmt.Sprintf("PluginConfiguration %q not found", e.Name)
}

// IsPluginConfigurationNotFound checks if the error indicates a missing PluginConfiguration.
func IsPluginConfigurationNotFound(err error) bool {
	var notFoundErr *PluginConfigurationNotFoundError

	return errors.As(err, &notFoundErr)
}

// GetServerSecretVolumeMountPath returns the volume mount path for server secrets.
func GetServerSecretVolumeMountPath(hostName string) string {
	return "/server-certs/" + hostName
}

// GetClientSecretVolumeMountPath returns the volume mount path for client secrets.
func GetClientSecretVolumeMountPath(hostName string) string {
	return "/client-certs/" + hostName
}
