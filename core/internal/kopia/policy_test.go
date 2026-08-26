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
	"strings"
	"testing"
)

// assertArgContains fails the test when args does not contain want.
func assertArgContains(t *testing.T, args []string, want string) {
	t.Helper()
	if !slices.Contains(args, want) {
		t.Errorf("expected args to contain %q, got %v", want, args)
	}
}

// assertNoArgWithPrefix fails the test when any arg starts with prefix.
func assertNoArgWithPrefix(t *testing.T, args []string, prefix string) {
	t.Helper()
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			t.Errorf("expected no arg with prefix %q, got %v", prefix, args)
		}
	}
}

func TestBuildCompressionPolicyArgs(t *testing.T) {
	t.Run("per-source target", func(t *testing.T) {
		target := Target{Username: "user", Hostname: "cluster"}
		args := buildCompressionPolicyArgs("/etc/kopia/config", target.String(),
			CompressionPolicy{Algorithm: "zstd"})

		assertArgContains(t, args, "--compression=zstd")
		assertArgContains(t, args, "--config-file=/etc/kopia/config")
		assertArgContains(t, args, "user@cluster")
		assertNoArgWithPrefix(t, args, "--global")
	})

	t.Run("global target", func(t *testing.T) {
		args := buildCompressionPolicyArgs("/etc/kopia/config", "--global",
			CompressionPolicy{Algorithm: "s2-default"})

		assertArgContains(t, args, "--compression=s2-default")
		assertArgContains(t, args, "--global")
	})

	t.Run("min and max size flags", func(t *testing.T) {
		args := buildCompressionPolicyArgs("/etc/kopia/config", "--global",
			CompressionPolicy{Algorithm: "zstd", MinSize: 4096, MaxSize: 1048576})

		assertArgContains(t, args, "--compression-min-size=4096")
		assertArgContains(t, args, "--compression-max-size=1048576")
	})

	t.Run("unset fields emit no flag", func(t *testing.T) {
		args := buildCompressionPolicyArgs("/etc/kopia/config", "--global",
			CompressionPolicy{MinSize: 4096})

		assertNoArgWithPrefix(t, args, "--compression=")
		assertNoArgWithPrefix(t, args, "--compression-max-size=")
		assertArgContains(t, args, "--compression-min-size=4096")
	})

	t.Run("last argument is the target", func(t *testing.T) {
		args := buildCompressionPolicyArgs("/etc/kopia/config", "--global",
			CompressionPolicy{Algorithm: "none"})
		if got := args[len(args)-1]; got != "--global" {
			t.Errorf("expected the target to be the last argument, got %q in %v", got, args)
		}
	})
}
