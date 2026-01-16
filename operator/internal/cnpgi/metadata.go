package cnpgi

import "github.com/cloudnative-pg/cnpg-i/pkg/identity"

// PluginName is the name of the plugin from the instance manager
// Point-of-view.
const PluginName = "klio.cnpg.io"

// data is the metadata of this plugin.
var data = identity.GetPluginMetadataResponse{ //nolint: gochecknoglobals
	Name:          PluginName,
	Version:       "0.0.11", // x-release-please-version
	DisplayName:   "Klio",
	ProjectUrl:    "",
	RepositoryUrl: "",
	License:       "TODO",
	LicenseUrl:    "TODO",
	Maturity:      "alpha",
}
