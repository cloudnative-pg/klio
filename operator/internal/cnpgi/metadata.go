package cnpgi

import (
	"github.com/cloudnative-pg/cnpg-i/pkg/identity"

	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

// data is the metadata of this plugin.
var data = identity.GetPluginMetadataResponse{ //nolint: gochecknoglobals
	Name:          klioconfig.PluginName,
	Version:       "0.0.17", // x-release-please-version
	DisplayName:   "Klio",
	ProjectUrl:    "",
	RepositoryUrl: "",
	License:       "TODO",
	LicenseUrl:    "TODO",
	Maturity:      "alpha",
}
