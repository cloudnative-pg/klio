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

package cnpgi

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudnative-pg/cnpg-i/pkg/wal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func TestAvailableTiers(t *testing.T) {
	const (
		tier1Addr = "klio:52000"
		tier2Addr = "klio:52001"
	)

	tests := []struct {
		name string
		cfg  config.Data
		want []tier
	}{
		{
			name: "standard mode, no tier2",
			cfg: config.Data{
				Tier1Enabled: true,
				Client:       config.ClientConfig{Wal: config.WalRepositoryClientConfig{Address: tier1Addr}},
			},
			want: []tier{tier1},
		},
		{
			name: "standard mode, tier2 backup only — restore must not use tier2",
			cfg: config.Data{
				Tier1Enabled:       true,
				Tier2BackupEnabled: true,
				Client: config.ClientConfig{Wal: config.WalRepositoryClientConfig{
					Address:      tier1Addr,
					Tier2Address: tier2Addr,
				}},
			},
			want: []tier{tier1},
		},
		{
			name: "standard mode, tier2 recovery only",
			cfg: config.Data{
				Tier1Enabled:         true,
				Tier2RecoveryEnabled: true,
				Client: config.ClientConfig{Wal: config.WalRepositoryClientConfig{
					Address:      tier1Addr,
					Tier2Address: tier2Addr,
				}},
			},
			want: []tier{tier1, tier2},
		},
		{
			name: "standard mode, tier2 backup and recovery",
			cfg: config.Data{
				Tier1Enabled:         true,
				Tier2BackupEnabled:   true,
				Tier2RecoveryEnabled: true,
				Client: config.ClientConfig{Wal: config.WalRepositoryClientConfig{
					Address:      tier1Addr,
					Tier2Address: tier2Addr,
				}},
			},
			want: []tier{tier1, tier2},
		},
		{
			name: "read-only mode, tier2 recovery",
			cfg: config.Data{
				Tier2RecoveryEnabled: true,
				Client:               config.ClientConfig{Wal: config.WalRepositoryClientConfig{Tier2Address: tier2Addr}},
			},
			want: []tier{tier2},
		},
		{
			name: "Tier1Enabled false but address present — tier1 still selectable",
			cfg: config.Data{
				Tier1Enabled:         false,
				Tier2RecoveryEnabled: true,
				Client: config.ClientConfig{Wal: config.WalRepositoryClientConfig{
					Address:      tier1Addr,
					Tier2Address: tier2Addr,
				}},
			},
			want: []tier{tier1, tier2},
		},
		{
			name: "no addresses, flags ignored",
			cfg: config.Data{
				Tier1Enabled:         true,
				Tier2RecoveryEnabled: true,
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := availableTiers(&tt.cfg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("availableTiers() = %v, want %v", got, tt.want)
			}
		})
	}
}

// findInt64HistogramDataPoints returns the data points of the named Int64
// Histogram instrument, or nil when the instrument recorded nothing.
func findInt64HistogramDataPoints(
	rm metricdata.ResourceMetrics, name string,
) []metricdata.HistogramDataPoint[int64] {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if h, ok := m.Data.(metricdata.Histogram[int64]); ok {
					return h.DataPoints
				}
			}
		}
	}

	return nil
}

// TestRestoreRecordsEarlyFailure checks that a restore rejected before any tier
// is chosen is still measured. The duration is recorded from a defer, so the
// validation bail-outs at the top of Restore must produce a data point too,
// otherwise the failure rate panel would silently under-count. The tier and
// cluster name are unknown that early, and must be reported as "unknown" rather
// than as an empty attribute value.
func TestRestoreRecordsEarlyFailure(t *testing.T) {
	reader := setupTestMeter(t)

	w := newWalServiceImplementation(newGRPCClientManager(), WALCapabilityOptions{})

	// No source WAL name: Restore rejects the request before touching a tier.
	_, err := w.Restore(context.Background(), &wal.WALRestoreRequest{})
	require.Error(t, err)

	rm := collectOTelMetrics(t, reader)
	dps := findInt64HistogramDataPoints(rm, opentelemetry.PluginWalRestoreDurationMetric)
	require.Len(t, dps, 1, "an early failure must still record one restore duration")

	outcome, ok := dps[0].Attributes.Value("outcome")
	require.True(t, ok, "data point must carry an outcome attribute")
	assert.Equal(t, string(opentelemetry.OutcomeFailure), outcome.AsString())

	tier, ok := dps[0].Attributes.Value("tier")
	require.True(t, ok, "data point must carry a tier attribute")
	assert.Equal(t, string(tierUnknown), tier.AsString())

	cluster, ok := dps[0].Attributes.Value("cluster_name")
	require.True(t, ok, "data point must carry a cluster_name attribute")
	assert.Equal(t, unknownAttributeValue, cluster.AsString())
}

func TestGetConnectionErrors(t *testing.T) {
	tests := []struct {
		name       string
		writeFile  bool
		configYAML string
		tier       tier
		wantSubstr string
	}{
		{
			name:       "nonexistent config file",
			writeFile:  false,
			tier:       tier1,
			wantSubstr: "while loading config file",
		},
		{
			name:       "invalid YAML",
			writeFile:  true,
			configYAML: "{invalid",
			tier:       tier1,
			wantSubstr: "while decoding config file",
		},
		{
			name:      "unknown tier",
			writeFile: true,
			configYAML: `
client:
  cluster_name: test
  wal:
    address: "localhost:52000"
    tier2_address: "localhost:52001"
`,
			tier:       tier("tier3"),
			wantSubstr: `unknown tier "tier3"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newGRPCClientManager()

			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if tt.writeFile {
				if err := os.WriteFile(configPath, []byte(tt.configYAML), 0o600); err != nil {
					t.Fatalf("writing config file: %v", err)
				}
			}

			_, err := mgr.getClient(
				context.Background(),
				walRestoreOptions{
					configFile: configPath,
					targetTier: tt.tier,
				},
			)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error = %q, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}
