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
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// ErrNoCertificateFound is returned when a file contains no PEM-encoded
// certificate.
var ErrNoCertificateFound = errors.New("no certificate found in file")

// LeafFingerprint computes the lowercase hex SHA256 fingerprint of the
// first certificate in a PEM file, matching the format Kopia expects
// for server certificate pinning.
func LeafFingerprint(certPath string) (string, error) {
	pemBytes, err := os.ReadFile(certPath) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("while reading certificate file: %w", err)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("%w: %s", ErrNoCertificateFound, certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("while parsing certificate file: %w", err)
	}

	fingerprint := sha256.Sum256(cert.Raw)

	return hex.EncodeToString(fingerprint[:]), nil
}
