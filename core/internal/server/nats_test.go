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

package server

import (
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewNatsServiceLoopbackOnly asserts that the embedded NATS server is
// reachable on loopback only. nats-server derives both its listener address
// and ClientURL from Options.Host, so checking ClientURL avoids binding the
// fixed client port in a test.
func TestNewNatsServiceLoopbackOnly(t *testing.T) {
	nats, err := NewNatsService(t.TempDir())
	require.NoError(t, err)

	clientURL, err := url.Parse(nats.ClientURL())
	require.NoError(t, err)

	ip := net.ParseIP(clientURL.Hostname())
	require.NotNil(t, ip, "NATS host %q is not an IP address", clientURL.Hostname())
	assert.True(t, ip.IsLoopback(), "NATS listens on %s", clientURL.Host)
}
