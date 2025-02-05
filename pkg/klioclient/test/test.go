package test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
)

const fakeWalContent = "deadbeef"

type testingRepository struct {
	prefilledSnapshots int
	conn               common.WALClient
}

type repositoryCreatorFunction func(ctx context.Context, repoLabel string) (common.WALClient, error)

// BenchLookupSnapshots starts a benchmark that creates a new repo, fills it with fake
// WALs and then look them up.
func BenchLookupSnapshots(b *testing.B, creator repositoryCreatorFunction) {
	b.Helper()

	ctx := context.TODO()

	combinations := []int{
		100,
		200,
		500,
		1000,
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

func addFakeWals(ctx context.Context, repo common.WALClient, start int, count int) error {
	for i := range count {
		walName := fmt.Sprintf("%024X", start+i)
		err := repo.StoreWAL(ctx, walName, []byte(fakeWalContent))
		if err != nil {
			return fmt.Errorf("while generating fake wal: %w", err)
		}
	}

	return nil
}
