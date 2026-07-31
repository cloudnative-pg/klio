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
	"testing"
)

func TestSourceInfoString(t *testing.T) {
	cases := []struct {
		name     string
		input    SourceInfo
		expected string
	}{
		{
			name:     "Global source info",
			input:    SourceInfo{},
			expected: "(global)",
		},
		{
			name:     "User and host only",
			input:    SourceInfo{UserName: "user", Host: "host"},
			expected: "user@host",
		},
		{
			name:     "Full source info",
			input:    SourceInfo{UserName: "user", Host: "host", Path: "/my/path"},
			expected: "user@host:/my/path",
		},
		{
			name:     "Path only",
			input:    SourceInfo{Path: "/only/path"},
			expected: "@:/only/path",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.String()
			if got != tc.expected {
				t.Errorf("SourceInfo.Of() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestTargetString(t *testing.T) {
	target := Target{
		Username: "user",
		Hostname: "host",
	}

	expected := "user@host"
	if got := target.String(); got != expected {
		t.Errorf("Target.Of() = %q, want %q", got, expected)
	}
}
