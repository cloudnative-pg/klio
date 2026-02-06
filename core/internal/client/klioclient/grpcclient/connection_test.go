package grpcclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func BenchmarkLookupSnapshotsViaKlioServer(b *testing.B) {
	createTemporaryKlioRepo := func(ctx context.Context) (*TemporaryConnection, error) {
		conn, err := ConnectTemporary(
			ctx,
			log.GetLogger(),
			&config.ClientConfig{
				ClusterName: "cluster-name",
			},
			repository.Options{
				FS:       afero.NewMemMapFs(),
				Password: "random-string",
			},
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating local kopia repository: %w", err)
		}

		return conn, nil
	}

	BenchLookupSnapshots(b, createTemporaryKlioRepo)
}
