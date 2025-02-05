package kopia

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
	"github.com/EnterpriseDB/klio/pkg/klioclient/test"
)

func BenchmarkLookupSnapshotsViaKopia(b *testing.B) {
	createTemporaryKopiaRepo := func(ctx context.Context, repoLabel string) (common.Client, error) {
		//nolint:usetesting
		dirName, err := os.MkdirTemp(
			"",
			"kopia_"+repoLabel,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating local temporary directory: %w", err)
		}

		conn, err := ConnectTemporary(ctx, slog.Default(), LocalRepositoryOptions{
			Path:       dirName,
			Password:   "random-string",
			Hostname:   "bench",
			Username:   "bench",
			Initialize: true,
		})
		if err != nil {
			return nil, fmt.Errorf("error while creating local kopia repository at %v: %w", dirName, err)
		}

		return conn, nil
	}

	test.BenchLookupSnapshots(b, createTemporaryKopiaRepo)
}
