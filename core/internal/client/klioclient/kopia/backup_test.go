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

	"github.com/stretchr/testify/assert"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

func TestTablespaceTagValue(t *testing.T) {
	tests := []struct {
		name       string
		tablespace string
		want       string
	}{
		{name: "plain name", tablespace: "tbs1", want: "tbs1"},
		{name: "absolute path", tablespace: "/var/lib/postgresql/tbs1", want: "tbs1"},
		{name: "trailing slash", tablespace: "/var/lib/postgresql/tbs1/", want: "tbs1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tablespaceTagValue(klioclient.TablespaceLayout{Name: tt.tablespace})

			assert.Equal(t, tt.want, got)
		})
	}
}

// A tablespace named by its path is uploaded with the base name as tag, so
// the completeness check must look for that tag, not for the path.
func TestBackupPartsMissingTablespaceWithPath(t *testing.T) {
	parts := NewBackupParts([]kopia.Manifest{
		part("c1", "b1", "pgdata", ""),
		part("c1", "b1", "controldata", ""),
		part("c1", "b1", "tablespace", "tbs1"),
	})
	metadata := &klioclient.BackupMetadata{
		Name:        "b1",
		Tablespaces: []klioclient.TablespaceLayout{{Name: "/var/lib/postgresql/tbs1"}},
	}

	assert.Empty(t, parts.Missing("c1", "b1", metadata))
}
