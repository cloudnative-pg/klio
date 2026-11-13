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

package admin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

func TestListBackupsIncomplete(t *testing.T) {
	s := &Server{
		tier1: &mockClient{
			backups:    klioclient.BackupList{{ClusterName: "c1", Name: "b2"}},
			incomplete: []klioclient.IncompleteBackup{{ClusterName: "c1", BackupName: "b1", Latest: true}},
		},
		tier2: &mockClient{
			incomplete: []klioclient.IncompleteBackup{
				{ClusterName: "c1", BackupName: "b2", MetadataPresent: true, MissingParts: []string{"pgdata"}},
			},
		},
	}

	result, err := s.ListBackups(context.Background(), &klioGRPC.ListBackupsRequest{Incomplete: true})
	require.NoError(t, err)

	assert.JSONEq(t, `[
		{"clusterName": "c1", "backupName": "b1", "tier": "tier1", "status": "PENDING",
		 "startedAt": "", "metadataPresent": false, "missingParts": null},
		{"clusterName": "c1", "backupName": "b2", "tier": "tier2", "status": "RECOVERABLE",
		 "startedAt": "", "metadataPresent": true, "missingParts": ["pgdata"]}
	]`, string(result.GetBackupManifests()))
}

func TestListBackupsIncompleteEmptyIsAnArray(t *testing.T) {
	s := &Server{tier1: &mockClient{}}

	result, err := s.ListBackups(context.Background(), &klioGRPC.ListBackupsRequest{Incomplete: true})
	require.NoError(t, err)

	assert.Equal(t, "[]", string(result.GetBackupManifests()))
}

func TestListBackupsIncompleteNeedsTier1(t *testing.T) {
	s := &Server{tier2: &mockClient{}}

	_, err := s.ListBackups(context.Background(), &klioGRPC.ListBackupsRequest{Incomplete: true})

	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}
