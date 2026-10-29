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
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// incompleteKey is one series of the incomplete backups gauge.
type incompleteKey struct {
	cluster string
	tier    opentelemetry.Tier
	status  klioclient.IncompleteStatus
}

// sweepIncompleteBackups reports the incomplete backups of tc's tier: it
// logs them with their status and records the counts for the incomplete
// backups gauge. It never deletes one: that is left to the operator, with
// `klio admin delete-backup`.
func (s *Sweeper) sweepIncompleteBackups(ctx context.Context, tc tierConfig) error {
	backups, err := tc.backupClient.ListIncompleteBackups(ctx, "")
	if err != nil {
		return fmt.Errorf("while listing incomplete backups: %w", err)
	}

	if tc.metricTier == opentelemetry.Tier1 {
		backups = klioclient.ClassifyTier1IncompleteBackups(backups)
	} else {
		// A tier2 backup complete on tier1 is rebuilt by the next relay.
		tier1Complete, err := s.tier1Client.ListBackups(ctx, "")
		if err != nil {
			return fmt.Errorf("while listing tier1 backups: %w", err)
		}
		backups = klioclient.ClassifyTier2IncompleteBackups(backups, tier1Complete)
	}
	s.recordIncompleteBackups(tc.metricTier, backups)

	if len(backups) == 0 {
		return nil
	}

	names := make([]string, 0, len(backups))
	for _, b := range backups {
		names = append(names, fmt.Sprintf("%s/%s (%s)", b.ClusterName, b.BackupName, b.Status))
	}
	log.FromContext(ctx).Info("Incomplete backups found", "tier", tc.name, "backups", names)

	return nil
}

// recordIncompleteBackups replaces tier's series of the incomplete backups
// gauge with the counts of backups.
func (s *Sweeper) recordIncompleteBackups(tier opentelemetry.Tier, backups []klioclient.IncompleteBackup) {
	s.incompleteMu.Lock()
	defer s.incompleteMu.Unlock()

	for key := range s.incomplete {
		if key.tier == tier {
			delete(s.incomplete, key)
		}
	}
	for _, b := range backups {
		s.incomplete[incompleteKey{cluster: b.ClusterName, tier: tier, status: b.Status}]++
	}
}

// registerMetrics registers the incomplete backups gauge callback.
func (s *Sweeper) registerMetrics() error {
	_, err := otel.Meter(opentelemetry.Meter).RegisterCallback(
		s.observeIncompleteBackups,
		opentelemetry.ServerBackup.IncompleteBackups,
	)

	return err
}

// observeIncompleteBackups reports the counts of the last sweep.
func (s *Sweeper) observeIncompleteBackups(_ context.Context, o metric.Observer) error {
	s.incompleteMu.Lock()
	defer s.incompleteMu.Unlock()

	for key, count := range s.incomplete {
		o.ObserveInt64(opentelemetry.ServerBackup.IncompleteBackups, count, metric.WithAttributes(
			opentelemetry.AttributeKeyClusterName.Of(key.cluster),
			key.tier.Attribute(),
			opentelemetry.AttributeKeyStatus.Of(string(key.status)),
		))
	}

	return nil
}
