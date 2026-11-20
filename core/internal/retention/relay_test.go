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

package retention

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// relayCase is a cluster about to be relayed to tier2.
type relayCase struct {
	name string
	// tier2Latest is the tier2 retention; nil means no tier2 policy.
	tier2Latest *int32
	tier1       klioclient.BackupList
	tier2       klioclient.BackupList
	partial     []string
	// wantRelay are the backups to relay, oldest first.
	wantRelay []string
	// wantTier2 are the backups tier2 holds once the relay and the sweep
	// are done, oldest first.
	wantTier2 []string
}

func relayCases() []relayCase {
	backups := func(startedAt ...int64) klioclient.BackupList {
		list := make(klioclient.BackupList, 0, len(startedAt))
		for _, at := range startedAt {
			list = append(list, klioclient.BackupMetadata{Name: fmt.Sprintf("b%d", at), StartedAt: at})
		}

		return list
	}

	return []relayCase{
		{
			name:        "the new backup is relayed",
			tier2Latest: new(int32(2)),
			tier1:       backups(1, 2, 3),
			tier2:       backups(1, 2),
			wantRelay:   []string{"b3"},
			wantTier2:   []string{"b2", "b3"},
		},
		{
			// b1 and b2 were on tier2 and its sweep deleted them: copying
			// them back would only have the sweep delete them again.
			name:        "a backup removed by tier2 retention is not copied back",
			tier2Latest: new(int32(1)),
			tier1:       backups(1, 2, 3, 4),
			tier2:       backups(3),
			wantRelay:   []string{"b4"},
			wantTier2:   []string{"b4"},
		},
		{
			name:        "every backup pending after a tier2 outage is relayed",
			tier2Latest: new(int32(3)),
			tier1:       backups(1, 2, 3, 4),
			tier2:       backups(1),
			wantRelay:   []string{"b2", "b3", "b4"},
			wantTier2:   []string{"b2", "b3", "b4"},
		},
		{
			name:        "pending backups that tier2 retention would not keep are skipped",
			tier2Latest: new(int32(2)),
			tier1:       backups(2, 3, 4, 5, 6),
			tier2:       backups(1, 2),
			wantRelay:   []string{"b5", "b6"},
			wantTier2:   []string{"b5", "b6"},
		},
		{
			name:        "an older backup missing from tier2 is relayed when retention keeps it",
			tier2Latest: new(int32(3)),
			tier1:       backups(1, 2, 3),
			tier2:       backups(2, 3),
			wantRelay:   []string{"b1"},
			wantTier2:   []string{"b1", "b2", "b3"},
		},
		{
			// Completing it lets the sweep delete it: an incomplete backup
			// is never deleted.
			name:        "a partial backup is completed even out of retention",
			tier2Latest: new(int32(1)),
			tier1:       backups(1, 2, 3),
			tier2:       backups(2),
			partial:     []string{"b1"},
			wantRelay:   []string{"b1", "b3"},
			wantTier2:   []string{"b3"},
		},
		{
			name:      "a partial backup no longer complete on tier1 is not relayed",
			tier1:     backups(2),
			tier2:     backups(2),
			partial:   []string{"b1"},
			wantRelay: []string{},
			wantTier2: []string{"b2"},
		},
		{
			name:        "retention disabled relays everything missing",
			tier2Latest: new(int32(0)),
			tier1:       backups(1, 2, 3),
			tier2:       backups(2),
			wantRelay:   []string{"b1", "b3"},
			wantTier2:   []string{"b1", "b2", "b3"},
		},
		{
			name:      "no tier2 policy relays everything missing",
			tier1:     backups(3, 1, 2),
			wantRelay: []string{"b1", "b2", "b3"},
			wantTier2: []string{"b1", "b2", "b3"},
		},
		{
			name:        "nothing to relay",
			tier2Latest: new(int32(2)),
			tier1:       backups(1, 2),
			tier2:       backups(1, 2),
			wantRelay:   []string{},
			wantTier2:   []string{"b1", "b2"},
		},
	}
}

func (c relayCase) tier2Policy() *grpc.TierRetentionPolicy {
	if c.tier2Latest == nil {
		return nil
	}

	return &grpc.TierRetentionPolicy{Latest: *c.tier2Latest}
}

func TestBackupsToRelay(t *testing.T) {
	for _, tt := range relayCases() {
		t.Run(tt.name, func(t *testing.T) {
			tier1, tier2 := slices.Clone(tt.tier1), slices.Clone(tt.tier2)

			got, err := BackupsToRelay(tt.tier2Policy(), tier1, tier2, tt.partial)
			if err != nil {
				t.Fatalf("BackupsToRelay() error = %v", err)
			}
			if !slices.Equal(got, tt.wantRelay) {
				t.Fatalf("BackupsToRelay() = %v, want %v", got, tt.wantRelay)
			}

			sameName := func(a, b klioclient.BackupMetadata) bool { return a.Name == b.Name }
			if !slices.EqualFunc(tier1, tt.tier1, sameName) || !slices.EqualFunc(tier2, tt.tier2, sameName) {
				t.Fatalf("BackupsToRelay() reordered its inputs: tier1 %v, tier2 %v", tier1, tier2)
			}
		})
	}

	t.Run("a negative latest is rejected", func(t *testing.T) {
		_, err := BackupsToRelay(&grpc.TierRetentionPolicy{Latest: -1}, nil, nil, nil)
		if !errors.Is(err, ErrInvalidRetentionLatest) {
			t.Fatalf("BackupsToRelay() error = %v, want %v", err, ErrInvalidRetentionLatest)
		}
	})
}

// TestRelayedBackupsSurviveTheTier2Sweep lands the backups to relay on tier2
// in every order, with the tier2 sweep running after each one: the sweep
// must never delete a backup that tier2 keeps in the end, and tier2 must end up
// holding the same backups whatever the order.
func TestRelayedBackupsSurviveTheTier2Sweep(t *testing.T) {
	for _, tt := range relayCases() {
		t.Run(tt.name, func(t *testing.T) {
			relay, err := BackupsToRelay(tt.tier2Policy(), tt.tier1, tt.tier2, tt.partial)
			if err != nil {
				t.Fatalf("BackupsToRelay() error = %v", err)
			}

			for _, order := range permutations(relay) {
				tier2 := slices.Clone(tt.tier2)

				for _, name := range order {
					landed := slices.IndexFunc(tt.tier1, func(b klioclient.BackupMetadata) bool { return b.Name == name })
					tier2 = append(tier2, tt.tier1[landed])

					for _, deleted := range sweepTier2(t, tt.tier2Policy(), tier2) {
						if slices.Contains(tt.wantTier2, deleted) {
							t.Fatalf("order %v: the sweep deleted %q after %q landed", order, deleted, name)
						}

						tier2 = slices.DeleteFunc(tier2, func(b klioclient.BackupMetadata) bool { return b.Name == deleted })
					}
				}

				tier2.SortByAscendingTime()
				names := make([]string, 0, len(tier2))
				for _, b := range tier2 {
					names = append(names, b.Name)
				}
				if !slices.Equal(names, tt.wantTier2) {
					t.Fatalf("order %v: tier2 holds %v, want %v", order, names, tt.wantTier2)
				}
			}
		})
	}
}

// sweepTier2 returns the names of the complete tier2 backups the sweep
// deletes, using the same policy evaluation as sweepCluster.
func sweepTier2(t *testing.T, tier2Policy *grpc.TierRetentionPolicy, complete klioclient.BackupList) []string {
	t.Helper()

	// No policy, or latest: 0, disables retention: sweepCluster returns
	// before evaluating anything.
	if tier2Policy.GetLatest() == 0 {
		return nil
	}

	evaluator, err := policyFromProto(tier2Policy)
	if err != nil {
		t.Fatalf("policyFromProto() error = %v", err)
	}

	toDelete, err := deletionCandidates(context.Background(), "cluster-a", tierConfig{name: "tier2"},
		&grpc.RetentionPolicy{Tier2Policy: tier2Policy}, evaluator, complete)
	if err != nil {
		t.Fatalf("deletionCandidates() error = %v", err)
	}

	names := make([]string, 0, len(toDelete))
	for _, b := range toDelete {
		names = append(names, b.Name)
	}

	return names
}

// permutations returns every ordering of names.
func permutations(names []string) [][]string {
	if len(names) <= 1 {
		return [][]string{slices.Clone(names)}
	}

	var result [][]string
	for i, name := range names {
		rest := slices.Concat(names[:i], names[i+1:])
		for _, tail := range permutations(rest) {
			result = append(result, append([]string{name}, tail...))
		}
	}

	return result
}
