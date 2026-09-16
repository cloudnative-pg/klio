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

package kopia

import (
	"slices"
	"testing"
)

func TestBuildRetentionPolicyArgs(t *testing.T) {
	args := buildRetentionPolicyArgs("/etc/kopia/config", "--global", "0")

	for _, counter := range kopiaRetentionCounters() {
		assertArgContains(t, args, counter+"=0")
	}
	assertArgContains(t, args, "--config-file=/etc/kopia/config")
	if args[len(args)-1] != "--global" {
		t.Errorf("expected the target as last arg, got %v", args)
	}
}

func TestParsePolicyTargets(t *testing.T) {
	// Trimmed `kopia policy list --json` output: the global policy plus a
	// per-source one, with the unrelated policy fields left in place.
	raw := []byte(`[
	  {"id":"global","target":{"host":"","userName":"","path":""},"retention":{"keepLatest":10}},
	  {"id":"abc","target":{"host":"cluster","userName":"user","path":"/pgdata"},"compression":{}},
	  {"id":"def","target":{"host":"cluster","userName":"user","path":""}}
	]`)

	targets, err := parsePolicyTargets(raw)
	if err != nil {
		t.Fatalf("parsePolicyTargets() error = %v", err)
	}

	var got []string
	for _, target := range targets {
		if target.isGlobal() {
			continue
		}
		got = append(got, target.String())
	}

	want := []string{"user@cluster:/pgdata", "user@cluster"}
	if !slices.Equal(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}

	if _, err := parsePolicyTargets([]byte("not json")); err == nil {
		t.Error("expected an error on malformed output")
	}
}
