package server

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/server/admin"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// AdminServer manages the Klio admin server lifecycle and configuration.
type AdminServer struct {
	Config *config.ServerConfig

	Tier1KopiaConfigFile string
	Tier2KopiaConfigFile string
	SocketPath           string
	RunID                string
	RunSecret            string
}

// Serve starts the admin server and blocks until the context is canceled.
func (a *AdminServer) Serve(ctx context.Context) error {
	certificateFingerprint, err := kopia.ExtractSHA256CertificateFingerprint(
		a.Config.TLS.TLSCert)
	if err != nil {
		return fmt.Errorf("error while extracting fingerprint of the kopia server certificate: %w", err)
	}

	opts := admin.Options{
		Tier1KopiaConfigFile:   a.Tier1KopiaConfigFile,
		Tier2KopiaConfigFile:   a.Tier2KopiaConfigFile,
		SocketPath:             a.SocketPath,
		RunID:                  a.RunID,
		RunSecret:              a.RunSecret,
		CertificateFingerprint: certificateFingerprint,
		Tier2ServerAddress:     "https://" + a.Config.Tier2.BaseListenAddress,
	}

	server, err := admin.New(opts)
	if err != nil {
		return err
	}

	return server.Start(ctx)
}

func (a *AdminServer) String() string {
	return "admin-server"
}
