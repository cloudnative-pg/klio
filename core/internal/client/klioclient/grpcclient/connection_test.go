package grpcclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/server/walserver/repository"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func BenchmarkLookupSnapshotsViaKlioServer(b *testing.B) {
	createTemporaryKlioRepo := func(ctx context.Context) (*TemporaryConnection, error) {
		dirName := b.TempDir()

		conn, err := ConnectTemporary(
			ctx,
			log.GetLogger(),
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
