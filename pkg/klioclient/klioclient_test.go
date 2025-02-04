package klioclient

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/EnterpriseDB/klio/internal/klioserver/repository"
	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient/grpcclient"
	"github.com/EnterpriseDB/klio/pkg/klioclient/kopia"
)

const fakeWalContent = "deadbeef"

type testingRepository struct {
	prefilledSnapshots int
	conn               Client
}

type repositoryCreatorFunction func(ctx context.Context, repoLabel string) (Client, error)

func BenchmarkLookupSnapshotsViaKopia(b *testing.B) {
	createTemporaryKopiaRepo := func(ctx context.Context, repoLabel string) (Client, error) {
		//nolint:usetesting
		dirName, err := os.MkdirTemp(
			"",
			"kopia_"+repoLabel,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating local temporary directory: %w", err)
		}

		conn, err := kopia.ConnectTemporary(ctx, slog.Default(), kopia.LocalRepositoryOptions{
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

	benchLookupSnapshots(b, createTemporaryKopiaRepo)
}

func BenchmarkLookupSnapshotsViaKlioServer(b *testing.B) {
	createTemporaryKlioRepo := func(_ context.Context, repoLabel string) (Client, error) {
		//nolint:usetesting
		dirName, err := os.MkdirTemp(
			"",
			"klio_"+repoLabel,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating local temporary directory: %w", err)
		}

		conn, err := grpcclient.ConnectTemporary(
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

	benchLookupSnapshots(b, createTemporaryKlioRepo)
}

func benchLookupSnapshots(b *testing.B, creator repositoryCreatorFunction) {
	b.Helper()

	ctx := context.TODO()

	combinations := []int{
		100,
		200,
		500,
		1000,
		5000,
	}

	repositories := make([]testingRepository, len(combinations))
	for repoIdx, combination := range combinations {
		conn, err := creator(ctx, fmt.Sprintf("repo_%v_*", combination))
		if err != nil {
			b.Fatalf("Error while creating repositories: %v", err)
		}

		err = addFakeWals(ctx, conn, 0, combination)
		if err != nil {
			b.Fatalf("error while creating snapshots: %v", err)
		}

		repositories[repoIdx] = testingRepository{
			prefilledSnapshots: combination,
			conn:               conn,
		}
	}

	b.Cleanup(func() {
		for i := range repositories {
			if err := repositories[i].conn.Close(ctx); err != nil {
				b.Fatalf("error while closing temporary repository: %v", err.Error())
			}
		}
	})

	for i := range repositories {
		b.Run(fmt.Sprintf("lookup-%v", repositories[i].prefilledSnapshots), func(b *testing.B) {
			runSnapshotLookupBenchmark(b, &repositories[i])
		})
	}
}

func runSnapshotLookupBenchmark(b *testing.B, repo *testingRepository) {
	b.Helper()

	for range b.N {
		//nolint:gosec
		walName := fmt.Sprintf("%024X", rand.IntN(repo.prefilledSnapshots))
		content, err := repo.conn.GetWAL(context.TODO(), walName)
		if err != nil {
			b.Fatalf("error while looking up WAL %v: %v", walName, err)
		}

		if len(content.Content()) != len(fakeWalContent) {
			b.Fatalf(
				"WAL has not the expected length: %v vs %v",
				len(content.Content()),
				len(fakeWalContent),
			)
		}
	}
}

func addFakeWals(ctx context.Context, repo Client, start int, count int) error {
	for i := range count {
		walName := fmt.Sprintf("%024X", start+i)
		err := repo.StoreWAL(ctx, walName, []byte(fakeWalContent))
		if err != nil {
			return fmt.Errorf("while generating fake wal: %w", err)
		}
	}

	return nil
}
