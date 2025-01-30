package klioclient

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"testing"
)

const fakeWalContent = "deadbeef"

type TestingRepository struct {
	prefilledSnapshots int
	conn               *Connection
	directory          string
}

func BenchmarkLookupSnapshots(b *testing.B) {
	ctx := context.TODO()
	combinations := []int{
		100,
		200,
		500,
		1000,
	}

	repositories := make([]TestingRepository, len(combinations))
	for repoIdx, combination := range combinations {
		directory, conn, err := createRepoWithSnapshots(ctx, combination)
		if err != nil {
			b.Fatalf("Error while creating repositories: %v", err)
		}

		repositories[repoIdx] = TestingRepository{
			prefilledSnapshots: combination,
			conn:               conn,
			directory:          directory,
		}
	}

	defer func() {
		for i := range repositories {
			_ = repositories[i].conn.Close(ctx)
			_ = os.RemoveAll(repositories[i].directory)
		}
	}()

	for i := range repositories {
		b.Run(fmt.Sprintf("lookup-%v", repositories[i].prefilledSnapshots), func(b *testing.B) {
			runSnapshotLookupBenchmark(b, &repositories[i])
		})

		// b.Run(fmt.Sprintf("put-after-%v", repositories[i].prefilledSnapshots), func(b *testing.B) {
		// 	runSnapshotPutBenchmark(b, &repositories[i])
		// })
	}
}

func createRepoWithSnapshots(ctx context.Context, howManyShapshots int) (string, *Connection, error) {
	dirName, err := os.MkdirTemp(
		"",
		fmt.Sprintf("kopia_repo_%v_*", howManyShapshots),
	)
	if err != nil {
		return "", nil, fmt.Errorf("error while creating local temporary directory: %w", err)
	}

	conn, err := ConnectLocal(ctx, slog.Default(), LocalRepositoryOptions{
		Path:       dirName,
		Password:   "random-string",
		Hostname:   "bench",
		Username:   "bench",
		Initialize: true,
	})
	if err != nil {
		return "", nil, fmt.Errorf("error while creating local kopia repository at %v: %w", dirName, err)
	}

	err = addFakeWals(ctx, conn, 0, howManyShapshots)
	if err != nil {
		return "", nil, fmt.Errorf("error while creating snapshots: %w", err)
	}

	return dirName, conn, nil
}

func runSnapshotLookupBenchmark(b *testing.B, repo *TestingRepository) {
	b.Helper()

	for range b.N {
		//nolint:gosec
		walName := fmt.Sprintf("%024X", rand.IntN(repo.prefilledSnapshots))
		content, err := repo.conn.GetWAL(context.TODO(), walName)
		if err != nil {
			b.Fatalf("error while looking up WAL %v: %v", walName, err)
		}

		if len(content.content) != len(fakeWalContent) {
			b.Fatalf(
				"WAL has not the expected length: %v vs %v",
				len(content.content),
				len(fakeWalContent),
			)
		}
	}
}

// func runSnapshotPutBenchmark(b *testing.B, repo *TestingRepository) {
// 	err := addFakeWals(context.TODO(), repo.conn, repo.prefilledSnapshots, b.N)
// 	if err != nil {
// 		b.Fatalf("error while creating snapshots: %v", err)
// 	}
// }

func addFakeWals(ctx context.Context, repo *Connection, start int, count int) error {
	for i := range count {
		walName := fmt.Sprintf("%024X", start+i)
		err := repo.StoreWAL(ctx, walName, []byte(fakeWalContent))
		if err != nil {
			return err
		}
	}

	return nil
}
