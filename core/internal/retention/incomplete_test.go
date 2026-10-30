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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

func TestSweepIncompleteBackupsTier1(t *testing.T) {
	deleter := &stubDeleter{incomplete: []klioclient.IncompleteBackup{
		{ClusterName: "c1", BackupName: "b1"},
		{ClusterName: "c1", BackupName: "b2", Latest: true},
	}}
	s := &Sweeper{incomplete: make(map[incompleteKey]int64)}

	err := s.sweepIncompleteBackups(context.Background(),
		tierConfig{name: "tier1", metricTier: opentelemetry.Tier1, backupClient: deleter})
	require.NoError(t, err)

	assert.Empty(t, deleter.deleted, "incomplete backups are never deleted by the sweep")
	assert.Equal(t, map[incompleteKey]int64{
		{cluster: "c1", tier: opentelemetry.Tier1, status: klioclient.IncompleteStatusUnrecoverable}: 1,
		{cluster: "c1", tier: opentelemetry.Tier1, status: klioclient.IncompleteStatusPending}:       1,
	}, s.incomplete)
}

func TestSweepIncompleteBackupsTier2(t *testing.T) {
	deleter := &stubDeleter{incomplete: []klioclient.IncompleteBackup{
		{ClusterName: "c1", BackupName: "b1", Latest: true},
		{ClusterName: "c1", BackupName: "b2"},
	}}
	s := &Sweeper{
		incomplete:  make(map[incompleteKey]int64),
		tier1Client: &stubDeleter{complete: klioclient.BackupList{{ClusterName: "c1", Name: "b1"}}},
	}

	err := s.sweepIncompleteBackups(context.Background(),
		tierConfig{name: "tier2", metricTier: opentelemetry.Tier2, backupClient: deleter})
	require.NoError(t, err)

	assert.Empty(t, deleter.deleted, "incomplete backups are never deleted by the sweep")
	assert.Equal(t, map[incompleteKey]int64{
		{cluster: "c1", tier: opentelemetry.Tier2, status: klioclient.IncompleteStatusUnrecoverable}: 1,
		{cluster: "c1", tier: opentelemetry.Tier2, status: klioclient.IncompleteStatusRecoverable}:   1,
	}, s.incomplete)
}

func TestSweepIncompleteBackupsForgetsPreviousCounts(t *testing.T) {
	s := &Sweeper{incomplete: map[incompleteKey]int64{
		{cluster: "c1", tier: opentelemetry.Tier1, status: klioclient.IncompleteStatusPending}: 1,
		{cluster: "c1", tier: opentelemetry.Tier2, status: klioclient.IncompleteStatusPending}: 1,
	}}

	err := s.sweepIncompleteBackups(context.Background(),
		tierConfig{name: "tier1", metricTier: opentelemetry.Tier1, backupClient: &stubDeleter{}})
	require.NoError(t, err)

	// Only tier1's counts are replaced.
	assert.Equal(t, map[incompleteKey]int64{
		{cluster: "c1", tier: opentelemetry.Tier2, status: klioclient.IncompleteStatusPending}: 1,
	}, s.incomplete)
}
