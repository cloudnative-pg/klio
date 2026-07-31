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
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
)

func TestLoadEncryptionKey(t *testing.T) {
	encPath, idPath := ageEncrypt(t, t.TempDir(), "test-key")

	cfg := &Tier1Config{EncryptionKeyFile: encPath, IdentityFile: idPath}
	if err := cfg.LoadEncryptionKey(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EncryptionKey != "test-key" {
		t.Errorf("got %q, want %q", cfg.EncryptionKey, "test-key")
	}
}

// ageEncrypt creates an Age-encrypted file and identity file in dir.
// Returns (encryptedFilePath, identityFilePath).
func ageEncrypt(t *testing.T, dir, plaintext string) (string, string) {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generating Age identity: %v", err)
	}

	// Write identity file
	identityPath := filepath.Join(dir, "identity.txt")
	identityContent := fmt.Sprintf("# public key: %s\n%s\n", identity.Recipient(), identity)
	if err := os.WriteFile(identityPath, []byte(identityContent), 0o600); err != nil {
		t.Fatalf("writing identity file: %v", err)
	}

	// Encrypt the plaintext
	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, identity.Recipient())
	if err != nil {
		t.Fatalf("creating Age encryptor: %v", err)
	}
	if _, err := io.WriteString(writer, plaintext); err != nil {
		t.Fatalf("writing plaintext: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing Age encryptor: %v", err)
	}

	encryptedPath := filepath.Join(dir, "key.age")
	if err := os.WriteFile(encryptedPath, encrypted.Bytes(), 0o600); err != nil {
		t.Fatalf("writing encrypted file: %v", err)
	}

	return encryptedPath, identityPath
}

// ageEncryptArmored creates an ASCII-armored Age-encrypted file and identity file in dir.
// Returns (encryptedFilePath, identityFilePath).
func ageEncryptArmored(t *testing.T, dir, plaintext string) (string, string) {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generating Age identity: %v", err)
	}

	// Write identity file
	identityPath := filepath.Join(dir, "identity.txt")
	identityContent := fmt.Sprintf("# public key: %s\n%s\n", identity.Recipient(), identity)
	if err := os.WriteFile(identityPath, []byte(identityContent), 0o600); err != nil {
		t.Fatalf("writing identity file: %v", err)
	}

	// Encrypt the plaintext with armor
	var encrypted bytes.Buffer
	armorWriter := armor.NewWriter(&encrypted)
	writer, err := age.Encrypt(armorWriter, identity.Recipient())
	if err != nil {
		t.Fatalf("creating Age encryptor: %v", err)
	}
	if _, err := io.WriteString(writer, plaintext); err != nil {
		t.Fatalf("writing plaintext: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing Age encryptor: %v", err)
	}
	if err := armorWriter.Close(); err != nil {
		t.Fatalf("closing armor writer: %v", err)
	}

	encryptedPath := filepath.Join(dir, "key.age")
	if err := os.WriteFile(encryptedPath, encrypted.Bytes(), 0o600); err != nil {
		t.Fatalf("writing encrypted file: %v", err)
	}

	return encryptedPath, identityPath
}

func TestDecryptEncryptionKeyFile(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (encPath, idPath string)
		wantKey   string
		wantError bool
	}{
		{
			name: "Decrypts binary format successfully",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				return ageEncrypt(t, t.TempDir(), "my-secret-key\n")
			},
			wantKey: "my-secret-key",
		},
		{
			name: "Decrypts armored format successfully",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				return ageEncryptArmored(t, t.TempDir(), "armored-secret-key\n")
			},
			wantKey: "armored-secret-key",
		},
		{
			name: "Wrong identity fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				encPath, _ := ageEncrypt(t, dir, "my-secret-key")
				otherIdentity, _ := age.GenerateX25519Identity()
				wrongIDPath := filepath.Join(dir, "wrong-identity.txt")
				if err := os.WriteFile(wrongIDPath, []byte(otherIdentity.String()+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}

				return encPath, wrongIDPath
			},
			wantError: true,
		},
		{
			name: "Missing identity file fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				encPath, _ := ageEncrypt(t, t.TempDir(), "key")

				return encPath, "/nonexistent/identity"
			},
			wantError: true,
		},
		{
			name: "Missing encrypted file fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				_, idPath := ageEncrypt(t, t.TempDir(), "key")

				return "/nonexistent/key.age", idPath
			},
			wantError: true,
		},
		{
			name: "Plaintext file with identity fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				_, idPath := ageEncrypt(t, dir, "dummy")
				plaintextPath := filepath.Join(dir, "plaintext-key")
				if err := os.WriteFile(plaintextPath, []byte("not-age-encrypted"), 0o600); err != nil {
					t.Fatal(err)
				}

				return plaintextPath, idPath
			},
			wantError: true,
		},
		{
			name: "Decrypted empty content fails",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				return ageEncrypt(t, t.TempDir(), "   \n  ")
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encPath, idPath := tt.setup(t)
			key, err := DecryptEncryptionKeyFile(encPath, idPath)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tt.wantKey {
				t.Errorf("got %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestDecryptRejectsOtherReadableIdentity(t *testing.T) {
	dir := t.TempDir()
	encPath, idPath := ageEncrypt(t, dir, "key")

	if err := os.Chmod(idPath, 0o644); err != nil { //nolint:gosec // intentionally insecure for test
		t.Fatal(err)
	}

	_, err := DecryptEncryptionKeyFile(encPath, idPath)
	if err == nil {
		t.Error("expected error for other-readable identity, got nil")
	}
}

func TestDecryptAcceptsGroupReadableIdentity(t *testing.T) {
	dir := t.TempDir()
	encPath, idPath := ageEncrypt(t, dir, "my-key")

	// Kubernetes Secret volumes with DefaultMode 0400 often get 0440
	// due to fsGroup. This must be accepted.
	if err := os.Chmod(idPath, 0o440); err != nil { //nolint:gosec // intentionally group-readable for test
		t.Fatal(err)
	}

	key, err := DecryptEncryptionKeyFile(encPath, idPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "my-key" {
		t.Errorf("got %q, want %q", key, "my-key")
	}
}

func TestLoadEncryptionKeyWithAge(t *testing.T) {
	t.Run("Tier1", func(t *testing.T) {
		encPath, idPath := ageEncrypt(t, t.TempDir(), "age-decrypted-key")
		cfg := &Tier1Config{
			EncryptionKeyFile: encPath,
			IdentityFile:      idPath,
		}
		if err := cfg.LoadEncryptionKey(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.EncryptionKey != "age-decrypted-key" {
			t.Errorf("got %q, want %q", cfg.EncryptionKey, "age-decrypted-key")
		}
	})

	t.Run("Tier2", func(t *testing.T) {
		encPath, idPath := ageEncrypt(t, t.TempDir(), "age-tier2-key")
		cfg := &Tier2Config{
			EncryptionKeyFile: encPath,
			IdentityFile:      idPath,
		}
		if err := cfg.LoadEncryptionKey(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.EncryptionKey != "age-tier2-key" {
			t.Errorf("got %q, want %q", cfg.EncryptionKey, "age-tier2-key")
		}
	})
}
