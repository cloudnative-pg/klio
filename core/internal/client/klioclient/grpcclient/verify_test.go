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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
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

func writeTestServerCert(
	t *testing.T, caCertPEM, caKeyDER []byte, serverName string, serial int64,
) *x509.Certificate {
	t.Helper()

	serverCert, _ := newTestServerKeyPair(t, caCertPEM, caKeyDER, serverName, serial)

	return serverCert
}

func newTestServerKeyPair(
	t *testing.T, caCertPEM, caKeyDER []byte, serverName string, serial int64,
) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	caBlock, _ := pem.Decode(caCertPEM)
	require.NotNil(t, caBlock)

	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	require.NoError(t, err)

	caKey, err := x509.ParseECPrivateKey(caKeyDER)
	require.NoError(t, err)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: serverName},
		DNSNames:     []string{serverName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
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
	serverCert := writeTestServerCert(t, caCertPEM, caKeyDER, testServerHostname, 2)

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
	}, &testPKI{caCertPEM: caCertPEM, serverCert: serverCert}
}

func verifyConnectionState(
	t *testing.T, tlsConfig *tls.Config, serverName string, peerCertificates []*x509.Certificate,
) error {
	t.Helper()
	require.NotNil(t, tlsConfig.VerifyConnection)

	return tlsConfig.VerifyConnection(tls.ConnectionState{
		ServerName:       serverName,
		PeerCertificates: peerCertificates,
	})
}

func TestVerifyConnectionOK(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)
	require.NoError(t, verifyConnectionState(t, tlsConfig, testServerHostname, []*x509.Certificate{pki.serverCert}))
}

func TestVerifyConnectionRotatedBundle(t *testing.T) {
	dir := t.TempDir()
	clientConfig, _ := writeTestClientConfig(t, dir)

	// Build against the first CA, then rotate both the bundle file and
	// the presented certificate: the new handshake must trust the new
	// bundle without rebuilding the TLS configuration.
	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)

	caCertPEM, caKeyDER := writeTestCAPrivate(t, "test-server-ca", 10)
	serverCert := writeTestServerCert(t, caCertPEM, caKeyDER, testServerHostname, 11)
	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, caCertPEM, 0o600))

	require.NoError(t, verifyConnectionState(t, tlsConfig, testServerHostname, []*x509.Certificate{serverCert}))
}

func TestVerifyConnectionWrongCA(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	otherCAPEM, otherCAKey := writeTestCAPrivate(t, "other-ca", 20)
	otherServerCert := writeTestServerCert(t, otherCAPEM, otherCAKey, testServerHostname, 21)

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)

	require.NoError(t, verifyConnectionState(t, tlsConfig, testServerHostname, []*x509.Certificate{pki.serverCert}))
	require.Error(t, verifyConnectionState(t, tlsConfig, testServerHostname, []*x509.Certificate{otherServerCert}))
}

func TestVerifyConnectionWrongHostname(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)
	require.Error(t, verifyConnectionState(t, tlsConfig, "other-name", []*x509.Certificate{pki.serverCert}))
}

func TestVerifyConnectionMissingBundle(t *testing.T) {
	dir := t.TempDir()
	clientConfig, pki := writeTestClientConfig(t, dir)

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)
	require.NoError(t, os.Remove(clientConfig.Wal.ServerCertPath))

	require.Error(t, verifyConnectionState(t, tlsConfig, testServerHostname, []*x509.Certificate{pki.serverCert}))
}

func TestVerifyConnectionEmptyChain(t *testing.T) {
	clientConfig, _ := writeTestClientConfig(t, t.TempDir())

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)

	err = verifyConnectionState(t, tlsConfig, testServerHostname, nil)
	require.ErrorIs(t, err, ErrNoServerCertificate)
}

func TestVerifyConnectionEmptyServerName(t *testing.T) {
	clientConfig, pki := writeTestClientConfig(t, t.TempDir())

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)

	err = verifyConnectionState(t, tlsConfig, "", []*x509.Certificate{pki.serverCert})
	require.ErrorIs(t, err, ErrNoServerName)
}

func TestVerifyConnectionWrongKeyUsage(t *testing.T) {
	clientConfig, _ := writeTestClientConfig(t, t.TempDir())

	// A certificate valid only for client authentication must not be
	// accepted as a server certificate.
	caCertPEM, caKeyDER := writeTestCAPrivate(t, "test-server-ca", 30)
	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, caCertPEM, 0o600))
	clientOnly := newTestCertWithUsage(t, caCertPEM, caKeyDER, x509.ExtKeyUsageClientAuth)

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)
	require.Error(t, verifyConnectionState(t, tlsConfig, testServerHostname, []*x509.Certificate{clientOnly}))
}

func TestVerifyConnectionIntermediate(t *testing.T) {
	clientConfig, _ := writeTestClientConfig(t, t.TempDir())

	rootPEM, rootKeyDER := writeTestCAPrivate(t, "test-root-ca", 40)
	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, rootPEM, 0o600))

	// An intermediate CA signed by the root, and a leaf signed by it:
	// only the root is in the bundle, the chain comes from the peer.
	intermediate, intermediateKey := newTestIntermediate(t, rootPEM, rootKeyDER)
	leaf := newTestLeafFromIntermediate(t, intermediate, intermediateKey)

	tlsConfig, err := buildTLSConfig(clientConfig)
	require.NoError(t, err)
	require.NoError(t, verifyConnectionState(t, tlsConfig, testServerHostname,
		[]*x509.Certificate{leaf, intermediate}))
	require.Error(t, verifyConnectionState(t, tlsConfig, testServerHostname,
		[]*x509.Certificate{leaf}))
}

func TestBuildTLSConfigBadBundle(t *testing.T) {
	dir := t.TempDir()
	clientConfig, _ := writeTestClientConfig(t, dir)

	missing := &config.ClientConfig{
		ClusterName: "test-cluster",
		Wal:         clientConfig.Wal,
	}
	missing.Wal.ServerCertPath = filepath.Join(dir, "absent.crt")

	_, err := buildTLSConfig(missing)
	require.Error(t, err)

	require.NoError(t, os.WriteFile(clientConfig.Wal.ServerCertPath, []byte("not a bundle"), 0o600))

	_, err = buildTLSConfig(clientConfig)
	require.ErrorIs(t, err, ErrInconsistentCertificate)
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
) *x509.Certificate {
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

	return cert
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
) *x509.Certificate {
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

	return cert
}
