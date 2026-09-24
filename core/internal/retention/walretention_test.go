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
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

func TestFindOldestWAL(t *testing.T) {
	tests := []struct {
		name    string
		backups klioclient.BackupList
		want    string
	}{
		{
			name:    "empty list",
			backups: nil,
			want:    "",
		},
		{
			name: "all empty StartWAL",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: ""},
				{Name: "b2", StartWAL: ""},
			},
			want: "",
		},
		{
			name: "single entry",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: "000000010000000000000005"},
			},
			want: "000000010000000000000005",
		},
		{
			name: "already sorted ascending",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: "000000010000000000000001"},
				{Name: "b2", StartWAL: "000000010000000000000005"},
				{Name: "b3", StartWAL: "00000001000000000000000A"},
			},
			want: "000000010000000000000001",
		},
		{
			name: "unsorted, mixed with empty StartWAL entries",
			backups: klioclient.BackupList{
				{Name: "b1", StartWAL: "000000010000000000000005"},
				{Name: "b2", StartWAL: ""},
				{Name: "b3", StartWAL: "000000010000000000000002"},
				{Name: "b4", StartWAL: "00000001000000000000000A"},
			},
			want: "000000010000000000000002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findOldestWAL(tt.backups); got != tt.want {
				t.Fatalf("findOldestWAL() = %q, want %q", got, tt.want)
			}
		})
	}
}
