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

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

const testServerHostname = "klio-server"

type testPKI struct {
	caCertPEM  []byte
	serverCert *x509.Certificate
	serverKey  *ecdsa.PrivateKey
}

func writeTestCAPrivate(t *testing.T, commonName string, serial int64) ([]byte, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	encodedKey, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: parsed.Raw}), encodedKey
}

// newTestServerKeyPair issues a server certificate for serverName, as an IP
// SAN when serverName is an IP address and as a DNS SAN otherwise.
func newTestServerKeyPair(
	t *testing.T, caCertPEM, caKeyDER []byte, serverName string, serial int64,
) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	caCert, caKey := parseTestCA(t, caCertPEM, caKeyDER)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(serverName); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{serverName}
	}

	der, err := x509.CreateCertificate(
		rand.Reader, template, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	serverCert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return serverCert, serverKey
}

func writeTestClientConfig(t *testing.T, dir string) (*config.ClientConfig, *testPKI) {
	t.Helper()

	caCertPEM, caKeyDER := writeTestCAPrivate(t, "test-server-ca", 1)
	serverCert, serverKey := newTestServerKeyPair(t, caCertPEM, caKeyDER, testServerHostname, 2)

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDER, err := x509.CreateCertificate(
		rand.Reader, clientTemplate, clientTemplate, &clientKey.PublicKey, clientKey)
	require.NoError(t, err)

	serverCAPath := filepath.Join(dir, "server-ca.crt")
	clientCertPath := filepath.Join(dir, "client.crt")
	clientKeyPath := filepath.Join(dir, "client.key")
	require.NoError(t, os.WriteFile(serverCAPath, caCertPEM, 0o600))
	require.NoError(t, os.WriteFile(clientCertPath,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}), 0o600))
	require.NoError(t, os.WriteFile(clientKeyPath,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER}), 0o600))

	return &config.ClientConfig{
		ClusterName: "test-cluster",
		Wal: config.WalRepositoryClientConfig{
			ServerCertPath: serverCAPath,
			ClientCertPath: clientCertPath,
			ClientKeyPath:  clientKeyPath,
		},
	}, &testPKI{caCertPEM: caCertPEM, serverCert: serverCert, serverKey: serverKey}
}

// clientHandshake runs one client handshake through transportCredentials
// against a TLS server presenting chain, and returns the client-side error.
func clientHandshake(
	t *testing.T,
	transportCredentials *reloadingCredentials,
	authority string,
	key *ecdsa.PrivateKey,
	chain ...*x509.Certificate,
) error {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	rawChain := make([][]byte, 0, len(chain))
	for _, cert := range chain {
		rawChain = append(rawChain, cert.Raw)
	}

	go func() {
		serverConn, err := listener.Accept()
		if err != nil {
			return
		}

		server := tls.Server(serverConn, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"h2"},
			Certificates: []tls.Certificate{{Certificate: rawChain, PrivateKey: key}},
		})
		_ = server.HandshakeContext(context.Background())
		_ = server.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rawConn, err := (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
	require.NoError(t, err)

	conn, _, err := transportCredentials.ClientHandshake(ctx, authority, rawConn)
	if err != nil {
		_ = rawConn.Close()

		return err
	}

	return conn.Close()
}

func TestHandshakeOK(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)
	require.NoError(t, clientHandshake(t, transportCredentials, testServerHostname, pki.serverKey, pki.serverCert))
}

func TestHandshakeIPAddress(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	// Go never sends an IP address as SNI, so verification must use the
	// dialed address rather than the server name seen in the handshake.
	caKeyDER := rotateTestCA(t, clientConfig, pki, 5)
	serverCert, serverKey := newTestServerKeyPair(t, pki.caCertPEM, caKeyDER, "127.0.0.1", 6)

	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)
	require.NoError(t, clientHandshake(t, transportCredentials, "127.0.0.1:5000", serverKey, serverCert))
}

func TestHandshakeRotatedBundle(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	// Build against the first CA, then rotate both the bundle file and
	// the presented certificate: the new handshake must trust the new
	// bundle without rebuilding the credentials.
	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)

	caKeyDER := rotateTestCA(t, clientConfig, pki, 10)
	serverCert, serverKey := newTestServerKeyPair(t, pki.caCertPEM, caKeyDER, testServerHostname, 11)

	require.NoError(t, clientHandshake(t, transportCredentials, testServerHostname, serverKey, serverCert))
}

func TestHandshakeWrongCA(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	otherCAPEM, otherCAKey := writeTestCAPrivate(t, "other-ca", 20)
	otherServerCert, otherServerKey := newTestServerKeyPair(t, otherCAPEM, otherCAKey, testServerHostname, 21)

	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)

	require.NoError(t, clientHandshake(t, transportCredentials, testServerHostname, pki.serverKey, pki.serverCert))
	require.Error(t, clientHandshake(t, transportCredentials, testServerHostname, otherServerKey, otherServerCert))
}

func TestHandshakeWrongHostname(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)
	require.Error(t, clientHandshake(t, transportCredentials, "other-name", pki.serverKey, pki.serverCert))
}

func TestHandshakeMissingBundle(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)
	require.NoError(t, os.Remove(clientConfig.Wal.ServerCertPath))

	require.Error(t, clientHandshake(t, transportCredentials, testServerHostname, pki.serverKey, pki.serverCert))
}

func TestHandshakeWrongKeyUsage(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	// A certificate valid only for client authentication must not be
	// accepted as a server certificate.
	caKeyDER := rotateTestCA(t, clientConfig, pki, 30)
	clientOnly, clientOnlyKey := newTestCertWithUsage(t, pki.caCertPEM, caKeyDER, x509.ExtKeyUsageClientAuth)

	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)
	require.Error(t, clientHandshake(t, transportCredentials, testServerHostname, clientOnlyKey, clientOnly))
}

func TestHandshakeIntermediate(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	// An intermediate CA signed by the root, and a leaf signed by it:
	// only the root is in the bundle, the chain comes from the peer.
	rootKeyDER := rotateTestCA(t, clientConfig, pki, 40)
	intermediate, intermediateKey := newTestIntermediate(t, pki.caCertPEM, rootKeyDER)
	leaf, leafKey := newTestLeafFromIntermediate(t, intermediate, intermediateKey)

	transportCredentials, err := newReloadingCredentials(clientConfig)
	require.NoError(t, err)
	require.NoError(t, clientHandshake(t, transportCredentials, testServerHostname, leafKey, leaf, intermediate))
	require.Error(t, clientHandshake(t, transportCredentials, testServerHostname, leafKey, leaf))
}

func TestNewReloadingCredentialsBadBundle(t *testing.T) {
	dir := t.TempDir()
	clientConfig, _ := writeTestClientConfig(t, dir)

	missing := &config.ClientConfig{
		ClusterName: "test-cluster",
		Wal:         clientConfig.Wal,
	}
	missing.Wal.ServerCertPath = filepath.Join(dir, "absent.crt")

	_, err := newReloadingCredentials(missing)
	require.Error(t, err)

	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, []byte("not a bundle"), 0o600))

	_, err = newReloadingCredentials(clientConfig)
	require.ErrorIs(t, err, ErrInconsistentCertificate)
}

// rotateTestCA replaces the trust bundle on disk with a new CA, records it
// in pki, and returns the new CA key.
func rotateTestCA(t *testing.T, clientConfig *config.ClientConfig, pki *testPKI, serial int64) []byte {
	t.Helper()

	caCertPEM, caKeyDER := writeTestCAPrivate(t, "test-server-ca", serial)
	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, caCertPEM, 0o600))
	pki.caCertPEM = caCertPEM

	return caKeyDER
}

func parseTestCA(t *testing.T, caCertPEM, caKeyDER []byte) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	block, _ := pem.Decode(caCertPEM)
	require.NotNil(t, block)

	caCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	caKey, err := x509.ParseECPrivateKey(caKeyDER)
	require.NoError(t, err)

	return caCert, caKey
}

func newTestCertWithUsage(
	t *testing.T, caCertPEM, caKeyDER []byte, usage x509.ExtKeyUsage,
) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	caCert, caKey := parseTestCA(t, caCertPEM, caKeyDER)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(31),
		Subject:      pkix.Name{CommonName: testServerHostname},
		DNSNames:     []string{testServerHostname},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert, key
}

func newTestIntermediate(
	t *testing.T, rootPEM, rootKeyDER []byte,
) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	rootCert, rootKey := parseTestCA(t, rootPEM, rootKeyDER)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber:          big.NewInt(41),
		Subject:               pkix.Name{CommonName: "test-intermediate-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}, rootCert, &key.PublicKey, rootKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert, key
}

func newTestLeafFromIntermediate(
	t *testing.T, intermediate *x509.Certificate, intermediateKey *ecdsa.PrivateKey,
) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: testServerHostname},
		DNSNames:     []string{testServerHostname},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, intermediate, &key.PublicKey, intermediateKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert, key
}
