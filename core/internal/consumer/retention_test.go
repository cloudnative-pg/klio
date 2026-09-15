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

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/pkg/retention"
)

var errFakeDelete = errors.New("fake delete failure")

type fakeRetentionClient struct {
	backups klioclient.BackupList
	failOn  string
	deleted []string
}

func (f *fakeRetentionClient) DeleteBackup(_ context.Context, _ string, name string) error {
	if name == f.failOn {
		return errFakeDelete
	}
	f.deleted = append(f.deleted, name)

	return nil
}

func TestApplyRetention(t *testing.T) {
	catalog := klioclient.BackupList{
		{Name: "b1", StartedAt: 100},
		{Name: "b2", StartedAt: 200},
		{Name: "b3", StartedAt: 300},
	}

	tests := []struct {
		name        string
		client      *fakeRetentionClient
		policy      retention.Policy
		keep        func(*klioclient.BackupMetadata) bool
		wantDeleted []string
		wantErr     error
	}{
		{
			name:   "zero policy deletes nothing",
			client: &fakeRetentionClient{backups: catalog},
		},
		{
			name:        "latest keeps newest and deletes the rest",
			client:      &fakeRetentionClient{backups: catalog},
			policy:      retention.Policy{Latest: 1},
			wantDeleted: []string{"b1", "b2"},
		},
		{
			name:        "keep predicate protects an expired backup",
			client:      &fakeRetentionClient{backups: catalog},
			policy:      retention.Policy{Latest: 1},
			keep:        func(b *klioclient.BackupMetadata) bool { return b.Name == "b1" },
			wantDeleted: []string{"b2"},
		},
		{
			name:        "one failing delete does not stop the others",
			client:      &fakeRetentionClient{backups: catalog, failOn: "b1"},
			policy:      retention.Policy{Latest: 1},
			wantDeleted: []string{"b2"},
			wantErr:     errFakeDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Backup{}
			err := d.applyRetention(context.Background(), tt.client, "cluster", tt.client.backups, tt.policy, tt.keep)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("applyRetention() error = %v, want %v", err, tt.wantErr)
			}

			slices.Sort(tt.client.deleted)
			if !slices.Equal(tt.client.deleted, tt.wantDeleted) {
				t.Errorf("deleted = %v, want %v", tt.client.deleted, tt.wantDeleted)
			}
		})
	}
}
