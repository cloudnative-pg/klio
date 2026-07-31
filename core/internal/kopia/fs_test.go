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

func TestBuildConnectFSArgs(t *testing.T) {
	baseOpts := FSRepoOpts{
		CommonRepoOpts: CommonRepoOpts{
			CacheDirectory: "/cache",
		},
		DataDirectory: "/data/repo",
	}

	t.Run("ReadOnly true includes --readonly", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = true

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--readonly") {
			t.Errorf("expected args to contain --readonly, got %v", args)
		}
	})

	t.Run("ReadOnly false excludes --readonly", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = false

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if slices.Contains(args, "--readonly") {
			t.Errorf("expected args not to contain --readonly, got %v", args)
		}
	})

	t.Run("PersistCredentials true includes --persist-credentials", func(t *testing.T) {
		opts := baseOpts
		opts.PersistCredentials = true

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--persist-credentials") {
			t.Errorf("expected args to contain --persist-credentials, got %v", args)
		}
	})

	t.Run("PersistCredentials false excludes --persist-credentials", func(t *testing.T) {
		opts := baseOpts
		opts.PersistCredentials = false

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if slices.Contains(args, "--persist-credentials") {
			t.Errorf("expected args not to contain --persist-credentials, got %v", args)
		}
	})

	t.Run("both ReadOnly and PersistCredentials", func(t *testing.T) {
		opts := baseOpts
		opts.ReadOnly = true
		opts.PersistCredentials = true

		args := buildConnectFSArgs("/etc/kopia/config", opts)

		if !slices.Contains(args, "--readonly") {
			t.Errorf("expected args to contain --readonly, got %v", args)
		}
		if !slices.Contains(args, "--persist-credentials") {
			t.Errorf("expected args to contain --persist-credentials, got %v", args)
		}
	})

	t.Run("config file and data directory are included", func(t *testing.T) {
		opts := baseOpts

		args := buildConnectFSArgs("/custom/path/config.json", opts)

		if !slices.Contains(args, "--config-file=/custom/path/config.json") {
			t.Errorf("expected args to contain config file path, got %v", args)
		}
		if !slices.Contains(args, "--path=/data/repo") {
			t.Errorf("expected args to contain data directory path, got %v", args)
		}
	})
}
