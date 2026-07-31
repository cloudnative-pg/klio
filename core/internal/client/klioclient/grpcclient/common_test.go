/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package grpcclient

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
)

const fakeWalContent = "deadbeef"

// WALClient is the interface that wraps the backend WAL storage.
type WALClient interface {
	// StoreWAL upload a WAL file to a remote store
	StoreWAL(ctx context.Context, name string, content []byte, sendToTier2 bool) error
}

type testingRepository struct {
	prefilledSnapshots int
	client             *TemporaryConnection
}

type repositoryCreatorFunction func(ctx context.Context) (*TemporaryConnection, error)

// BenchLookupSnapshots starts a benchmark that creates a new repo, fills it with fake
// WALs and then look them up.
func BenchLookupSnapshots(b *testing.B, creator repositoryCreatorFunction) {
	b.Helper()

	ctx := b.Context()

	combinations := []int{
		100,
		200,
		500,
		1000,
	}

	repositories := make([]testingRepository, len(combinations))
	for repoIdx, combination := range combinations {
		client, err := creator(ctx)
		if err != nil {
			b.Fatalf("error while creating repositories: %v", err)
		}

		err = addFakeWals(ctx, client, 0, combination)
		if err != nil {
			b.Fatalf("error while creating snapshots: %v", err)
		}

		repositories[repoIdx] = testingRepository{
			prefilledSnapshots: combination,
			client:             client,
		}
	}

	b.Cleanup(func() {
		for i := range repositories {
			if err := repositories[i].client.Close(); err != nil {
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

		var buffer bytes.Buffer
		if err := repo.client.GetWALStreaming(b.Context(), walName, &buffer); err != nil {
			b.Fatalf("error while looking up WAL %v: %v", walName, err)
		}

		if buffer.Len() != len(fakeWalContent) {
			b.Fatalf(
				"WAL has not the expected length: %v vs %v",
				buffer.Len(),
				len(fakeWalContent),
			)
		}
	}
}

func addFakeWals(ctx context.Context, repo WALClient, start int, count int) error {
	for i := range count {
		walName := fmt.Sprintf("%024X", start+i)
		err := repo.StoreWAL(ctx, walName, []byte(fakeWalContent), false)
		if err != nil {
			return fmt.Errorf("while generating fake wal: %w", err)
		}
	}

	return nil
}
