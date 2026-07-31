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

const kopiaCheckForUpdatesEnv = "KOPIA_CHECK_FOR_UPDATES=false"

func TestEnvPassword(t *testing.T) {
	cases := []struct {
		name           string
		client         Client
		mustContain    []string
		mustNotContain []string
	}{
		{
			name: "Password provided",
			client: Client{
				Password: "password",
			},
			mustContain: []string{kopiaCheckForUpdatesEnv, "KOPIA_PASSWORD=password"}, // NOSONAR
		},
		{
			name: "Empty password",
			client: Client{
				Password: "",
			},
			mustContain:    []string{kopiaCheckForUpdatesEnv},
			mustNotContain: []string{"KOPIA_PASSWORD="},
		},
		{
			name: "Other fields set but no password",
			client: Client{
				ConfigFile:  "/path/to/config",
				KopiaBinary: "/path/to/kopia",
				Password:    "",
			},
			mustContain:    []string{kopiaCheckForUpdatesEnv},
			mustNotContain: []string{"KOPIA_PASSWORD="},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.client.kopiaEnvironmentVariables()

			for _, expected := range tc.mustContain {
				if !slices.Contains(got, expected) {
					t.Errorf("kopiaEnvironmentVariables() missing %q", expected)
				}
			}

			for _, notExpected := range tc.mustNotContain {
				for _, env := range got {
					if strings.HasPrefix(env, notExpected) {
						t.Errorf("kopiaEnvironmentVariables() unexpectedly contains %q", env)
					}
				}
			}
		})
	}
}
