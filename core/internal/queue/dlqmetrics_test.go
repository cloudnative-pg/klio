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

package queue

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// setupDLQTestMeter installs a MeterProvider backed by a ManualReader and
// rebinds the queue instruments against it, returning the reader.
func setupDLQTestMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = provider.Shutdown(context.Background())
		opentelemetry.InitServerQueueMetrics()
	})

	opentelemetry.InitServerQueueMetrics()

	return reader
}

// dlqCounts collects the int64 sum named metricName and returns its data
// points keyed by their sorted "key=value" attribute list.
func dlqCounts(t *testing.T, reader *sdkmetric.ManualReader, metricName string) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))

	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricName {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "%s must be an Int64 Sum", metricName)
			for _, dp := range sum.DataPoints {
				attrs := make([]string, 0, dp.Attributes.Len())
				for _, kv := range dp.Attributes.ToSlice() {
					attrs = append(attrs, string(kv.Key)+"="+kv.Value.AsString())
				}
				sort.Strings(attrs)
				out[strings.Join(attrs, ",")] = dp.Value
			}
		}
	}

	return out
}

// newDLQTestConn starts an embedded NATS server and returns a configured queue
// connection plus a JetStream handle on it.
//
//nolint:ireturn // returns the jetstream.JetStream interface by design
func newDLQTestConn(t *testing.T) (*Conn, jetstream.JetStream) {
	t.Helper()

	ns, url := startNATSServer(t)
	t.Cleanup(ns.Shutdown)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	conn, err := New(t.Context(), nc)
	require.NoError(t, err)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	return conn, js
}

func TestRetryFailedTasksRecordsRetriedEntries(t *testing.T) {
	reader := setupDLQTestMeter(t)
	conn, js := newDLQTestConn(t)
	ctx := t.Context()

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-a", "000000010000000000000002")
	seedFailedWAL(t, js, "cluster-b", "000000010000000000000001")
	// Two backup entries of one cluster are merged into a single task, but
	// both entries are retried.
	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-a")

	require.NoError(t, conn.RetryFailedWALTasks(ctx))
	require.NoError(t, conn.RetryFailedBackupTasks(ctx))

	assert.Equal(t, map[string]int64{
		"cluster_name=cluster-a,stream=klio-dlq-wal-stream":    2,
		"cluster_name=cluster-b,stream=klio-dlq-wal-stream":    1,
		"cluster_name=cluster-a,stream=klio-dlq-backup-stream": 2,
	}, dlqCounts(t, reader, opentelemetry.ServerQueueDLQRetriedMetric))
}

func TestDiscardFailedTasksRecordsDiscardReason(t *testing.T) {
	reader := setupDLQTestMeter(t)
	conn, js := newDLQTestConn(t)
	ctx := t.Context()

	seedFailedWAL(t, js, "cluster-a", testWALName)
	seedFailedWAL(t, js, "cluster-b", testWALName)
	seedFailedBackup(t, js, "cluster-a")
	seedFailedBackup(t, js, "cluster-b")

	// An operator discards cluster-a's entries...
	_, err := conn.DiscardFailedWALTasks(ctx, WithCluster("cluster-a"))
	require.NoError(t, err)
	_, err = conn.DiscardFailedBackupTasks(ctx, WithCluster("cluster-a"))
	require.NoError(t, err)

	// ...while cluster-b's are cleaned up after its tasks succeed.
	require.NoError(t, conn.purgeWALDLQEntries(ctx, "cluster-b", testWALName))
	require.NoError(t, conn.purgeBackupDLQEntries(ctx, "cluster-b"))

	assert.Equal(t, map[string]int64{
		"cluster_name=cluster-a,reason=manual,stream=klio-dlq-wal-stream":       1,
		"cluster_name=cluster-a,reason=manual,stream=klio-dlq-backup-stream":    1,
		"cluster_name=cluster-b,reason=automatic,stream=klio-dlq-wal-stream":    1,
		"cluster_name=cluster-b,reason=automatic,stream=klio-dlq-backup-stream": 1,
	}, dlqCounts(t, reader, opentelemetry.ServerQueueDLQDiscardedMetric))
}

// runDLQWatcher runs w against conn in the background and returns a function
// that stops it and waits for it to return.
func runDLQWatcher(t *testing.T, w *DLQWatcher, conn *Conn) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, conn) }()

	return func() {
		cancel()
		require.NoError(t, <-done)
	}
}

func TestDLQWatcherRecordsCapturedEntries(t *testing.T) {
	reader := setupDLQTestMeter(t)
	conn, js := newDLQTestConn(t)

	// An entry stored before the watcher's start time belongs to a previous
	// server run and must not be counted.
	seedFailedWAL(t, js, "old-cluster", testWALName)
	time.Sleep(10 * time.Millisecond)

	w := NewDLQWatcher(time.Now())
	stop := runDLQWatcher(t, w, conn)

	seedFailedWAL(t, js, "cluster-a", "000000010000000000000001")
	seedFailedWAL(t, js, "cluster-a", "000000010000000000000002")
	seedFailedBackup(t, js, "cluster-b")
	// An advisory whose original task is gone has no cluster to report.
	seedDLQAdvisory(t, js, klioWalStreamName, klioWalConsumerName, 9999)

	want := map[string]int64{
		"cluster_name=cluster-a,stream=klio-dlq-wal-stream":    2,
		"cluster_name=unknown,stream=klio-dlq-wal-stream":      1,
		"cluster_name=cluster-b,stream=klio-dlq-backup-stream": 1,
	}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, want, dlqCounts(t, reader, opentelemetry.ServerQueueDLQCapturedMetric))
	}, 5*time.Second, 50*time.Millisecond)

	// A restarted watcher resumes after the last counted entry.
	stop()
	seedFailedBackup(t, js, "cluster-b")
	stop = runDLQWatcher(t, w, conn)
	defer stop()

	want["cluster_name=cluster-b,stream=klio-dlq-backup-stream"] = 2
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, want, dlqCounts(t, reader, opentelemetry.ServerQueueDLQCapturedMetric))
	}, 5*time.Second, 50*time.Millisecond)
}
