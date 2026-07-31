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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// mockClient is a mock implementation of klioclient.Client for admin server tests.
type mockClient struct {
	klioclient.Client

	deleteBackupFunc func(ctx context.Context, hostname string, name string) error
}

func (m *mockClient) DeleteBackup(ctx context.Context, hostname string, name string) error {
	if m.deleteBackupFunc != nil {
		return m.deleteBackupFunc(ctx, hostname, name)
	}

	return nil
}

func (m *mockClient) Close(_ context.Context) {}

func (m *mockClient) ListBackups(_ context.Context, _ string) (klioclient.BackupList, error) {
	return nil, nil
}

func (m *mockClient) GetMetadata(_ context.Context, _ string, _ string) (*klioclient.BackupMetadata, error) {
	return nil, nil
}

func (m *mockClient) SetRetentionPolicy(_ context.Context, _ kopia.Target, _ kopia.RetentionPolicy) error {
	return nil
}

func (m *mockClient) GetRetentionPolicy(_ context.Context, _ kopia.Target) (*kopia.RetentionPolicy, error) {
	return nil, nil
}

func (m *mockClient) ApplyRetentionPolicy(_ context.Context, _ kopia.Target) error {
	return nil
}

func (m *mockClient) GetUsername() string { return "" }
func (m *mockClient) GetHostname() string { return "" }

func TestDeleteBackup(t *testing.T) {
	tests := []struct {
		name         string
		req          *klioGRPC.DeleteBackupRequest
		tier1        klioclient.Client
		tier2        klioclient.Client
		expectedCode codes.Code
		expectedMsg  string
	}{
		{
			name: "empty backup_name",
			req: &klioGRPC.DeleteBackupRequest{
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_1},
				ClusterName: "my-cluster",
			},
			tier1:        &mockClient{},
			expectedCode: codes.InvalidArgument,
			expectedMsg:  "backup_name must be specified",
		},
		{
			name: "empty cluster_name",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName: "backup-20260219",
				Tiers:      []klioGRPC.Tier{klioGRPC.Tier_TIER_1},
			},
			tier1:        &mockClient{},
			expectedCode: codes.InvalidArgument,
			expectedMsg:  "cluster_name must be specified",
		},
		{
			name: "no tiers specified",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{},
			},
			tier1:        &mockClient{},
			expectedCode: codes.InvalidArgument,
			expectedMsg:  "at least one tier must be specified",
		},
		{
			name: "TIER_UNSPECIFIED is rejected",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_UNSPECIFIED},
			},
			tier1:        &mockClient{},
			expectedCode: codes.InvalidArgument,
			expectedMsg:  "TIER_UNSPECIFIED is not a valid tier",
		},
		{
			name: "tier1 not configured",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_1},
			},
			tier1:        nil,
			expectedCode: codes.FailedPrecondition,
			expectedMsg:  "tier1 is not configured",
		},
		{
			name: "tier2 not configured",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_2},
			},
			tier1:        &mockClient{},
			tier2:        nil,
			expectedCode: codes.FailedPrecondition,
			expectedMsg:  "tier2 is not configured",
		},
		{
			name: "tier1 deletion error",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_1},
			},
			tier1: &mockClient{
				deleteBackupFunc: func(_ context.Context, _ string, _ string) error {
					return errors.New("kopia error")
				},
			},
			expectedCode: codes.Internal,
			expectedMsg:  "while deleting backup from tier1",
		},
		{
			name: "tier2 deletion error",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_2},
			},
			tier1: &mockClient{},
			tier2: &mockClient{
				deleteBackupFunc: func(_ context.Context, _ string, _ string) error {
					return errors.New("s3 error")
				},
			},
			expectedCode: codes.Internal,
			expectedMsg:  "while deleting backup from tier2",
		},
		{
			name: "successful tier1 deletion",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_1},
			},
			tier1:        &mockClient{},
			expectedCode: codes.OK,
		},
		{
			name: "successful tier2 deletion",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_2},
			},
			tier1:        &mockClient{},
			tier2:        &mockClient{},
			expectedCode: codes.OK,
		},
		{
			name: "successful both tiers deletion",
			req: &klioGRPC.DeleteBackupRequest{
				BackupName:  "backup-20260219",
				ClusterName: "my-cluster",
				Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_1, klioGRPC.Tier_TIER_2},
			},
			tier1:        &mockClient{},
			tier2:        &mockClient{},
			expectedCode: codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{
				tier1: tt.tier1,
				tier2: tt.tier2,
			}

			resp, err := srv.DeleteBackup(context.Background(), tt.req)

			if tt.expectedCode == codes.OK {
				require.NoError(t, err)
				assert.NotNil(t, resp)
			} else {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok, "error should be a gRPC status")
				assert.Equal(t, tt.expectedCode, st.Code())
				if tt.expectedMsg != "" {
					assert.Contains(t, st.Message(), tt.expectedMsg)
				}
			}
		})
	}
}

func TestDeleteBackupPassesCorrectArguments(t *testing.T) {
	var capturedHostname, capturedName string
	tier1Mock := &mockClient{
		deleteBackupFunc: func(_ context.Context, hostname string, name string) error {
			capturedHostname = hostname
			capturedName = name

			return nil
		},
	}

	srv := &Server{tier1: tier1Mock}

	_, err := srv.DeleteBackup(context.Background(), &klioGRPC.DeleteBackupRequest{
		BackupName:  "backup-20260219",
		ClusterName: "my-cluster",
		Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_1},
	})
	require.NoError(t, err)

	assert.Equal(t, "my-cluster", capturedHostname)
	assert.Equal(t, "backup-20260219", capturedName)
}

func TestDeleteBackupTier1FailureStopsTier2(t *testing.T) {
	tier2Called := false
	tier1Mock := &mockClient{
		deleteBackupFunc: func(_ context.Context, _ string, _ string) error {
			return errors.New("tier1 failure")
		},
	}
	tier2Mock := &mockClient{
		deleteBackupFunc: func(_ context.Context, _ string, _ string) error {
			tier2Called = true

			return nil
		},
	}

	srv := &Server{tier1: tier1Mock, tier2: tier2Mock}

	_, err := srv.DeleteBackup(context.Background(), &klioGRPC.DeleteBackupRequest{
		BackupName:  "backup-20260219",
		ClusterName: "my-cluster",
		Tiers:       []klioGRPC.Tier{klioGRPC.Tier_TIER_1, klioGRPC.Tier_TIER_2},
	})
	require.Error(t, err)
	assert.False(t, tier2Called, "tier2 should not be called when tier1 fails")
}
