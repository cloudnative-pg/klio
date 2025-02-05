package grpcclient

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/EnterpriseDB/klio/internal/klioserver/repository"
	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
	"github.com/EnterpriseDB/klio/pkg/klioclient/test"
)

func BenchmarkLookupSnapshotsViaKlioServer(b *testing.B) {
	createTemporaryKlioRepo := func(_ context.Context, repoLabel string) (common.Client, error) {
		//nolint:usetesting
		dirName, err := os.MkdirTemp(
			"",
			"klio_"+repoLabel,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating local temporary directory: %w", err)
		}

		conn, err := ConnectTemporary(
			slog.Default(),
			&config.KlioRepositoryClientConfig{
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

	test.BenchLookupSnapshots(b, createTemporaryKlioRepo)
}
