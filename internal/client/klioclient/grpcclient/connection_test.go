package grpcclient

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
	"github.com/EnterpriseDB/klio/pkg/config"
)

func BenchmarkLookupSnapshotsViaKlioServer(b *testing.B) {
	createTemporaryKlioRepo := func(_ context.Context) (*TemporaryConnection, error) {
		dirName := b.TempDir()

		conn, err := ConnectTemporary(
			slog.Default(),
			&config.WalRepositoryClientConfig{
				ClusterName: "cluster-name",
			},
			repository.Options{
				Path:     dirName,
				Password: "random-string",
			},
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating local kopia repository at %v: %w", dirName, err)
		}

		return conn, nil
	}

	BenchLookupSnapshots(b, createTemporaryKlioRepo)
}
