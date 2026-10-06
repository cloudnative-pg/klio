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

package grpcclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

// startTestTLSServer serves gRPC over TLS on addr with the given certificate.
func startTestTLSServer(
	t *testing.T, addr string, cert *x509.Certificate, key *ecdsa.PrivateKey,
) (string, *grpc.Server) {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	require.NoError(t, err)

	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{cert.Raw}, PrivateKey: key}},
	})))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return listener.Addr().String(), server
}

func waitForState(t *testing.T, conn *Connection, want connectivity.State) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		conn.grpcConnection.Connect()
		conn.grpcConnection.ResetConnectBackoff()

		state := conn.grpcConnection.GetState()
		if state == want {
			return
		}

		require.True(t, conn.grpcConnection.WaitForStateChange(ctx, state),
			"timed out waiting for %s, last state %s", want, state)
	}
}

// TestConnectHandshakeFollowsRotatedCA runs real gRPC handshakes: the server
// name comes from the dial address, and a reconnect after the server CA and
// certificate rotate must succeed without rebuilding the connection.
func TestConnectHandshakeFollowsRotatedCA(t *testing.T) {
	clientConfig, _ := writeTestClientConfig(t, t.TempDir())

	caPEM, caKeyDER := writeTestCAPrivate(t, "handshake-ca-1", 50)
	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, caPEM, 0o600))
	cert, key := newTestServerKeyPair(t, caPEM, caKeyDER, "localhost", 51)

	addr, firstServer := startTestTLSServer(t, "127.0.0.1:0", cert, key)
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	conn, err := Connect(clientConfig, net.JoinHostPort("localhost", port))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.grpcConnection.Close() })

	waitForState(t, conn, connectivity.Ready)

	// Rotate: new CA on disk, new server certificate on the same port.
	firstServer.Stop()

	newCAPEM, newCAKeyDER := writeTestCAPrivate(t, "handshake-ca-2", 52)
	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, newCAPEM, 0o600))
	newCert, newKey := newTestServerKeyPair(t, newCAPEM, newCAKeyDER, "localhost", 53)
	_, _ = startTestTLSServer(t, "127.0.0.1:"+port, newCert, newKey)

	waitForState(t, conn, connectivity.Ready)
}

// TestConnectHandshakeRejectsUntrustedServer checks that a server whose
// certificate is not signed by the bundle never reaches the Ready state.
func TestConnectHandshakeRejectsUntrustedServer(t *testing.T) {
	clientConfig, _ := writeTestClientConfig(t, t.TempDir())

	otherCAPEM, otherCAKeyDER := writeTestCAPrivate(t, "untrusted-ca", 60)
	cert, key := newTestServerKeyPair(t, otherCAPEM, otherCAKeyDER, "localhost", 61)
	addr, _ := startTestTLSServer(t, "127.0.0.1:0", cert, key)

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	conn, err := Connect(clientConfig, net.JoinHostPort("localhost", port))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.grpcConnection.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn.grpcConnection.Connect()

	for state := conn.grpcConnection.GetState(); state != connectivity.TransientFailure; {
		require.True(t, conn.grpcConnection.WaitForStateChange(ctx, state), "never failed, last state %s", state)
		state = conn.grpcConnection.GetState()
	}

	require.NotEqual(t, connectivity.Ready, conn.grpcConnection.GetState())
}
