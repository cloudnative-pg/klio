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

// Package queuetest provides shared helpers for tests that need a running
// NATS server with JetStream enabled.
package queuetest

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"
)

// StartNATSServer starts an embedded NATS server with JetStream enabled and
// returns its client URL. The server is shut down automatically when the
// test completes.
func StartNATSServer(t *testing.T) string {
	t.Helper()

	ns, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	require.NoError(t, err, "failed to create NATS server")

	go ns.Start()
	t.Cleanup(ns.Shutdown)

	require.True(t, ns.ReadyForConnections(4*time.Second), "NATS server not ready")

	return ns.ClientURL()
}
