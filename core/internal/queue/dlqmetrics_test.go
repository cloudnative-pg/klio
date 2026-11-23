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
	"errors"
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

// TestDLQCapturedCountsATaskFailingItsLastAttempt drives a WAL task through
// the production consumer until it exhausts its attempts, and checks it is
// counted once, on its last delivery.
func TestDLQCapturedCountsATaskFailingItsLastAttempt(t *testing.T) {
	reader := setupDLQTestMeter(t)
	conn, js := newDLQTestConn(t)
	ctx := t.Context()

	// Shrink the attempt budget of the WAL consumer, so the task is
	// dead-lettered in a fraction of a second instead of after the
	// production backoff schedule.
	_, err := js.UpdateConsumer(ctx, klioWalStreamName, jetstream.ConsumerConfig{
		Name:          klioWalConsumerName,
		Durable:       klioWalConsumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: 1,
		MaxDeliver:    2,
		AckWait:       200 * time.Millisecond,
	})
	require.NoError(t, err)

	go func() {
		_ = conn.ConsumeWALReceivedMessages(ctx, func(context.Context, *WALTask) error {
			return errors.New("tier2 unreachable")
		})
	}()

	task := WALTask{ClusterName: "cluster-a", WALName: testWALName}
	require.NoError(t, conn.notifyMessage(ctx, walSubject(task.ClusterName), task, nil))

	// JetStream dead-letters the task on its own: wait for the entry, so the
	// counter is compared with what really reached the dead-letter queue.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		failed, err := conn.ListFailedWALTasks(ctx)
		assert.NoError(c, err)
		assert.Len(c, failed, 1)
	}, 5*time.Second, 50*time.Millisecond)

	assert.Equal(t, map[string]int64{"cluster_name=cluster-a,stream=klio-dlq-wal-stream": 1},
		dlqCounts(t, reader, opentelemetry.ServerQueueDLQCapturedMetric))
}
