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

package kopia

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeFingerprintTestCert(t *testing.T, dir, name string) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	certPath := filepath.Join(dir, name+".crt")
	require.NoError(t, os.WriteFile(
		certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))

	return certPath
}

func TestLeafFingerprintTracksRotation(t *testing.T) {
	dir := t.TempDir()
	certPath := writeFingerprintTestCert(t, dir, "server")

	first, err := LeafFingerprint(certPath)
	require.NoError(t, err)
	require.Len(t, first, 2*sha256.Size)

	// Rewriting the file (a renewal) changes the fingerprint, and a
	// fresh read picks it up with no restart involved.
	_ = writeFingerprintTestCert(t, dir, "server")

	second, err := LeafFingerprint(certPath)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestLeafFingerprintFailures(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := LeafFingerprint(filepath.Join(t.TempDir(), "absent.crt"))
		require.Error(t, err)
	})

	t.Run("not a certificate", func(t *testing.T) {
		dir := t.TempDir()
		bundlePath := filepath.Join(dir, "ca.crt")
		require.NoError(t, os.WriteFile(bundlePath, []byte("not a bundle"), 0o600))

		_, err := LeafFingerprint(bundlePath)
		require.ErrorIs(t, err, ErrNoCertificateFound)
	})
}
