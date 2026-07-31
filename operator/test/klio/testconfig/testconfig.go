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

// Package testconfig loads e2e test configuration from a YAML file.
//
// Configuration is resolved in two layers, each overriding the previous:
//  1. Built-in defaults (always applied first)
//  2. YAML config file (default: e2e-config.yaml, override via E2E_CONFIG_FILE)
package testconfig

import (
	"errors"
	"os"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// Config value defaults.
const (
	defaultServerImage       = "registry.dev:5000/klio-testing:dev"
	defaultLogDir            = "e2e_cluster_logs"
	defaultOperatorNamespace = "cnpg-system"
	defaultOperatorAppLabel  = "app.kubernetes.io/name=klio"
)

// Config file loader settings.
const (
	defaultConfigFile = "e2e-config.yaml"
	envConfigFile     = "E2E_CONFIG_FILE"
)

// ImagePullSecretConfig holds registry credentials used to create a
// kubernetes.io/dockerconfigjson pull secret in each test namespace.
type ImagePullSecretConfig struct {
	// Registry is the registry hostname (e.g. "ghcr.io").
	Registry string `mapstructure:"registry"`
	// Username is the registry username.
	Username string `mapstructure:"username"`
	// Password is the registry password or token.
	Password string `mapstructure:"password"`
}

// IsConfigured returns true when all three credential fields are non-empty.
func (c ImagePullSecretConfig) IsConfigured() bool {
	return c.Registry != "" && c.Username != "" && c.Password != ""
}

// Config holds the e2e test configuration.
type Config struct {
	// ServerImage is the Klio server container image used in tests.
	ServerImage string `mapstructure:"serverImage"`

	// LogDir is the directory where pod logs are streamed during the test run.
	LogDir string `mapstructure:"logDir"`

	// StorageClass is the Kubernetes storage class used for all PVC templates
	// in the tests (tier1 cache, tier1 data, queue, tier2 cache).
	StorageClass string `mapstructure:"storageClass"`

	// OperatorNamespace is the namespace where the Klio operator runs. Defaults
	// to cnpg-system (the Helm/Kind layout); set to openshift-operators for the
	// OLM-based OpenShift install.
	OperatorNamespace string `mapstructure:"operatorNamespace"`

	// OperatorAppLabel is the label selector identifying the Klio operator
	// Deployment. Defaults to app.kubernetes.io/name=klio (Helm/Kind); the
	// OLM-based OpenShift install labels it app.kubernetes.io/name=klio-operator.
	OperatorAppLabel string `mapstructure:"operatorAppLabel"`

	// OperatorSubscription is the name of the OLM Subscription that manages the
	// operator, in OperatorNamespace. Set it on the OLM-based OpenShift install
	// so tests that need to change the operator's environment patch the
	// Subscription (which OLM propagates to the Deployment) instead of the
	// Deployment directly, which OLM reverts. Leave empty for the Helm/Kind
	// install, where the Deployment is patched directly.
	OperatorSubscription string `mapstructure:"operatorSubscription"`

	// ImagePullSecret holds optional registry credentials. When all fields are
	// non-empty, a pull secret named "e2e-pull-secret" is created in every test
	// namespace and referenced by the Server and Cluster templates.
	ImagePullSecret ImagePullSecretConfig `mapstructure:"imagePullSecret"`
}

// Load reads the e2e test configuration. It applies built-in defaults first,
// then overlays values from the YAML config file.
// If the config file does not exist, built-in defaults are used without error.
func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("serverImage", defaultServerImage)
	v.SetDefault("logDir", defaultLogDir)
	v.SetDefault("operatorNamespace", defaultOperatorNamespace)
	v.SetDefault("operatorAppLabel", defaultOperatorAppLabel)

	path := os.Getenv(envConfigFile)
	if path == "" {
		path = defaultConfigFile
	}
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DumpedKinds is the ordered set of object lists dumped for a namespace on
// failure. It covers the Klio and CloudNativePG custom resources together with
// the core workload objects needed to debug a failed e2e test. A fresh set is
// returned on each call so parallel features never share list objects.
func DumpedKinds() []k8s.ObjectList {
	return []k8s.ObjectList{
		&kliov1alpha1.ServerList{},
		&kliov1alpha1.PluginConfigurationList{},
		&cnpgv1.ClusterList{},
		&cnpgv1.BackupList{},
		&cnpgv1.ScheduledBackupList{},
		&corev1.PodList{},
		&corev1.PersistentVolumeClaimList{},
		&corev1.ServiceAccountList{},
		&corev1.EventList{},
		&appsv1.StatefulSetList{},
		&appsv1.DeploymentList{},
		&batchv1.JobList{},
		&discoveryv1.EndpointSliceList{},
	}
}
