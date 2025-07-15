package cnpgi

import "github.com/cloudnative-pg/cnpg-i/pkg/identity"

// data is the metadata of this plugin.
var data = identity.GetPluginMetadataResponse{ //nolint:gochecknoglobals
	Name:          "klio.cnpg.io",
	Version:       "0.0.1",
	DisplayName:   "Klio",
	ProjectUrl:    "",
	RepositoryUrl: "",
	License:       "",
	LicenseUrl:    "",
	Maturity:      "alpha",
}
