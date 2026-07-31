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
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
)

// UnexpectedBlockTypeError is raised when a certain PEM block type is not expected.
type UnexpectedBlockTypeError struct {
	FoundType    string
	ExpectedType string
}

// Error implements the error interface.
func (e *UnexpectedBlockTypeError) Error() string {
	return fmt.Sprintf("unexpected PEM block type, expected:%s found:%s", e.ExpectedType, e.FoundType)
}

// ExtractSHA256CertificateFingerprint extracts the SHA256 certificate fingerprint
// of the passed certificate file.
func ExtractSHA256CertificateFingerprint(certificateFile string) (string, error) {
	certPEMBlock, err := os.ReadFile(certificateFile) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("while reading the server certificate: %w", err)
	}

	block, _ := pem.Decode(certPEMBlock)
	if block.Type != "CERTIFICATE" {
		return "", &UnexpectedBlockTypeError{
			FoundType:    block.Type,
			ExpectedType: "CERTIFICATE",
		}
	}

	hasher := sha256.New()
	if _, err := hasher.Write(block.Bytes); err != nil {
		return "", fmt.Errorf("error while hashing server certificate: %w", err)
	}
	shaSum := hasher.Sum(nil)

	return hex.EncodeToString(shaSum), nil
}
