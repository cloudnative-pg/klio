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
	"reflect"
	"sort"
	"testing"
)

// names extracts and sorts the backup names of a catalog so expectations can be
// compared regardless of the order Evaluate returns them in.
func names(backups []Backup) []string {
	result := make([]string, 0, len(backups))
	for _, b := range backups {
		result = append(result, b.Name)
	}
	sort.Strings(result)

	return result
}

func TestEvaluateLatest(t *testing.T) {
	catalog := []Backup{
		{Name: "b1", StartedAt: 100},
		{Name: "b2", StartedAt: 200},
		{Name: "b3", StartedAt: 300},
		{Name: "b4", StartedAt: 400},
		{Name: "b5", StartedAt: 500},
	}

	tests := []struct {
		name        string
		catalog     []Backup
		policy      Policy
		wantExpired []string
	}{
		{
			name:        "zero policy keeps everything",
			catalog:     catalog,
			policy:      Policy{},
			wantExpired: []string{},
		},
		{
			name:        "keep more than available expires nothing",
			catalog:     catalog,
			policy:      Policy{Latest: 10},
			wantExpired: []string{},
		},
		{
			name:        "keep exactly the catalog size expires nothing",
			catalog:     catalog,
			policy:      Policy{Latest: 5},
			wantExpired: []string{},
		},
		{
			name:        "keep the two most recent",
			catalog:     catalog,
			policy:      Policy{Latest: 2},
			wantExpired: []string{"b1", "b2", "b3"},
		},
		{
			name:        "keep only the most recent",
			catalog:     catalog,
			policy:      Policy{Latest: 1},
			wantExpired: []string{"b1", "b2", "b3", "b4"},
		},
		{
			name:        "empty catalog expires nothing",
			catalog:     nil,
			policy:      Policy{Latest: 2},
			wantExpired: []string{},
		},
		{
			name:        "negative count keeps everything defensively",
			catalog:     catalog,
			policy:      Policy{Latest: -1},
			wantExpired: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(Evaluate(tt.catalog, tt.policy))
			if !reflect.DeepEqual(got, tt.wantExpired) {
				t.Fatalf("Evaluate() expired = %v, want %v", got, tt.wantExpired)
			}
		})
	}
}

// TestEvaluateOrdersByStartTime makes sure the survivors are chosen by
// recency even when the catalog is supplied out of order.
func TestEvaluateOrdersByStartTime(t *testing.T) {
	catalog := []Backup{
		{Name: "old", StartedAt: 100},
		{Name: "new", StartedAt: 300},
		{Name: "mid", StartedAt: 200},
	}

	got := names(Evaluate(catalog, Policy{Latest: 1}))
	want := []string{"mid", "old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Evaluate() expired = %v, want %v", got, want)
	}
}

// TestEvaluateDeterministicTie makes sure a shared start time yields a stable
// selection driven by the backup name.
func TestEvaluateDeterministicTie(t *testing.T) {
	catalog := []Backup{
		{Name: "a", StartedAt: 100},
		{Name: "b", StartedAt: 100},
		{Name: "c", StartedAt: 100},
	}

	got := names(Evaluate(catalog, Policy{Latest: 1}))
	// "c" sorts highest by name, so it survives; "a" and "b" expire.
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Evaluate() expired = %v, want %v", got, want)
	}
}

// TestEvaluateDoesNotMutateInput guards against the copy-on-sort contract.
func TestEvaluateDoesNotMutateInput(t *testing.T) {
	catalog := []Backup{
		{Name: "b1", StartedAt: 100},
		{Name: "b2", StartedAt: 200},
		{Name: "b3", StartedAt: 300},
	}
	before := make([]Backup, len(catalog))
	copy(before, catalog)

	_ = Evaluate(catalog, Policy{Latest: 1})

	if !reflect.DeepEqual(catalog, before) {
		t.Fatalf("Evaluate() mutated its input: got %v, want %v", catalog, before)
	}
}
