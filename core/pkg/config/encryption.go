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

package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// EmptyDecodedEncryptionKeyError is raised when the encryption key file exists
// but contains no key after decoding.
type EmptyDecodedEncryptionKeyError struct {
	filePath string
}

func (e *EmptyDecodedEncryptionKeyError) Error() string {
	return fmt.Sprintf("decoded encryption key file %s is empty", e.filePath)
}

// DecryptEncryptionKeyFile decrypts an Age-encrypted encryption key file
// using the provided Age identity file.
//
// The permissions of the identity file are not checked: a CSI driver decides
// them and cannot yet apply the pod fsGroup to them
// (https://github.com/kubernetes-sigs/secrets-store-csi-driver/pull/1841).
func DecryptEncryptionKeyFile(encryptionKeyFile, identityFile string) (string, error) {
	idFile, err := os.Open(identityFile) //nolint:gosec // path comes from validated config
	if err != nil {
		return "", fmt.Errorf("opening identity file: %w", err)
	}
	defer func() { _ = idFile.Close() }()

	identities, err := age.ParseIdentities(idFile)
	if err != nil {
		return "", fmt.Errorf("parsing identity file: %w", err)
	}
	encFile, err := os.Open(encryptionKeyFile) //nolint:gosec // path comes from validated config
	if err != nil {
		return "", fmt.Errorf("opening encryption key file: %w", err)
	}
	defer func() { _ = encFile.Close() }()

	// Try binary format first.
	reader, err := age.Decrypt(encFile, identities...)
	if err != nil {
		// Seek back and try ASCII-armored format.
		if _, seekErr := encFile.Seek(0, io.SeekStart); seekErr != nil {
			return "", fmt.Errorf("seeking encryption key file: %w", seekErr)
		}

		reader, err = age.Decrypt(armor.NewReader(encFile), identities...)
		if err != nil {
			return "", fmt.Errorf("decrypting encryption key file: %w", err)
		}
	}

	decrypted, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading decrypted encryption key: %w", err)
	}

	key := strings.TrimSpace(string(decrypted))
	if key == "" {
		return "", &EmptyDecodedEncryptionKeyError{filePath: encryptionKeyFile}
	}

	return key, nil
}

// LoadEncryptionKey reads and decrypts the encryption key from
// EncryptionKeyFile and populates the EncryptionKey field. Must be called at startup
// before any component accesses EncryptionKey.
func (c *Tier1Config) LoadEncryptionKey() error {
	key, err := DecryptEncryptionKeyFile(c.EncryptionKeyFile, c.IdentityFile)
	if err != nil {
		return err
	}
	c.EncryptionKey = key

	return nil
}

// LoadEncryptionKey reads and decrypts the encryption key from
// EncryptionKeyFile and populates the EncryptionKey field. Must be called at startup
// before any component accesses EncryptionKey.
func (c *Tier2Config) LoadEncryptionKey() error {
	key, err := DecryptEncryptionKeyFile(c.EncryptionKeyFile, c.IdentityFile)
	if err != nil {
		return err
	}
	c.EncryptionKey = key

	return nil
}
