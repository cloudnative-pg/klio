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

package consumer

import "testing"

func TestClampWAL(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		frontier  string
		expected  string
	}{
		{
			name:      "empty frontier skips deletion",
			requested: "000000010000000000000005",
			frontier:  "",
			expected:  "",
		},
		{
			name:      "frontier older than requested clamps to frontier",
			requested: "00000001000000000000000A",
			frontier:  "000000010000000000000005",
			expected:  "000000010000000000000005",
		},
		{
			name:      "frontier newer than requested keeps requested",
			requested: "000000010000000000000005",
			frontier:  "00000001000000000000000A",
			expected:  "000000010000000000000005",
		},
		{
			name:      "frontier equal to requested keeps requested",
			requested: "000000010000000000000007",
			frontier:  "000000010000000000000007",
			expected:  "000000010000000000000007",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampWAL(tt.requested, tt.frontier)
			if got != tt.expected {
				t.Errorf("clampWAL(%q, %q) = %q, want %q", tt.requested, tt.frontier, got, tt.expected)
			}
		})
	}
}
