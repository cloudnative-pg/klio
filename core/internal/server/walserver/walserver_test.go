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

package walserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

const testServerName = "klio-server"

type testIssuer struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// issue signs a certificate for the given usage. A nil issuer self-signs a CA.
func issue(t *testing.T, issuer *testIssuer, usage x509.ExtKeyUsage) (*testIssuer, tls.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: testServerName},
		DNSNames:     []string{testServerName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}

	parent, signer := template, key
	if issuer == nil {
		template.Subject.CommonName = "test-ca"
		template.IsCA = true
		template.BasicConstraintsValid = true
		template.KeyUsage = x509.KeyUsageCertSign
		template.ExtKeyUsage = nil
	} else {
		parent, signer = issuer.cert, issuer.key
	}

	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return &testIssuer{cert: cert, key: key}, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}
}

func writeCertificate(t *testing.T, path string, cert *x509.Certificate) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600))
}

func writeServerIdentity(t *testing.T, tlsConfiguration *config.TLSConfig, issuer *testIssuer) *x509.Certificate {
	t.Helper()

	server, _ := issue(t, issuer, x509.ExtKeyUsageServerAuth)
	keyDER, err := x509.MarshalECPrivateKey(server.key)
	require.NoError(t, err)

	writeCertificate(t, tlsConfiguration.TLSCert, server.cert)
	require.NoError(t, os.WriteFile(tlsConfiguration.TLSKey,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))

	return server.cert
}

// serverHandshake runs one mutual TLS handshake between a client presenting
// clientCert and the WAL server credentials, and returns the certificate the
// server presented, or the server-side handshake error.
func serverHandshake(
	t *testing.T, serverTLS *tls.Config, serverCA *x509.Certificate, clientCert tls.Certificate,
) (*x509.Certificate, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	roots := x509.NewCertPool()
	roots.AddCert(serverCA)

	presented := make(chan *x509.Certificate, 1)
	go func() {
		defer close(presented)

		clientConn, err := (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
		if err != nil {
			return
		}

		client := tls.Client(clientConn, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"h2"},
			ServerName:   testServerName,
			RootCAs:      roots,
			Certificates: []tls.Certificate{clientCert},
		})
		if err := client.HandshakeContext(ctx); err == nil {
			presented <- client.ConnectionState().PeerCertificates[0]
		}
		_ = client.Close()
	}()

	serverConn, err := listener.Accept()
	require.NoError(t, err)

	// The production path: the gRPC credentials wrap GetConfigForClient.
	conn, _, err := credentials.NewTLS(serverTLS).ServerHandshake(serverConn)
	if err != nil {
		_ = serverConn.Close()

		return nil, err
	}
	defer func() { _ = conn.Close() }()

	select {
	case cert := <-presented:
		return cert, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestReloadingTLSConfigFollowsRotation(t *testing.T) {
	dir := t.TempDir()
	tlsConfiguration := &config.TLSConfig{
		TLSCert:          filepath.Join(dir, "tls.crt"),
		TLSKey:           filepath.Join(dir, "tls.key"),
		ClientCACertFile: filepath.Join(dir, "client-ca.crt"),
	}

	serverCA, _ := issue(t, nil, 0)
	clientCA, _ := issue(t, nil, 0)
	_, clientCert := issue(t, clientCA, x509.ExtKeyUsageClientAuth)

	first := writeServerIdentity(t, tlsConfiguration, serverCA)
	writeCertificate(t, tlsConfiguration.ClientCACertFile, clientCA.cert)

	serverTLS := newReloadingTLSConfig(t.Context(), tlsConfiguration)

	presented, err := serverHandshake(t, serverTLS, serverCA.cert, clientCert)
	require.NoError(t, err)
	require.Equal(t, first.Raw, presented.Raw)

	t.Run("a renewed serving pair is presented on the next handshake", func(t *testing.T) {
		second := writeServerIdentity(t, tlsConfiguration, serverCA)

		presented, err := serverHandshake(t, serverTLS, serverCA.cert, clientCert)
		require.NoError(t, err)
		require.Equal(t, second.Raw, presented.Raw)
	})

	t.Run("a rotated client CA rejects clients of the old one", func(t *testing.T) {
		newClientCA, _ := issue(t, nil, 0)
		writeCertificate(t, tlsConfiguration.ClientCACertFile, newClientCA.cert)

		_, err := serverHandshake(t, serverTLS, serverCA.cert, clientCert)
		require.Error(t, err)

		_, newClientCert := issue(t, newClientCA, x509.ExtKeyUsageClientAuth)
		_, err = serverHandshake(t, serverTLS, serverCA.cert, newClientCert)
		require.NoError(t, err)
	})

	t.Run("an unreadable serving pair fails the handshake closed", func(t *testing.T) {
		require.NoError(t, os.WriteFile(tlsConfiguration.TLSKey, []byte("not a key"), 0o600))

		_, err := serverHandshake(t, serverTLS, serverCA.cert, clientCert)
		require.Error(t, err)
	})
}
