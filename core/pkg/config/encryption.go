package config

import (
	"fmt"
	"io"
	"io/fs"
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

// InsecureIdentityFileError is raised when the identity file is
// accessible to others (world-readable).
type InsecureIdentityFileError struct {
	filePath string
	mode     fs.FileMode
}

func (e *InsecureIdentityFileError) Error() string {
	return fmt.Sprintf("identity file %s must not be accessible by others, permission is %04o", e.filePath, e.mode)
}

// checkIdentityFilePermissions verifies that the identity file is not
// world-readable. The check uses 0o007 (others) rather than 0o077
// (group+others) because Kubernetes Secret volumes often set group-read
// (e.g., 0440) due to fsGroup even when DefaultMode is 0400.
func checkIdentityFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("checking identity file permissions: %w", err)
	}

	if info.Mode().Perm()&0o007 != 0 {
		return &InsecureIdentityFileError{filePath: path, mode: info.Mode().Perm()}
	}

	return nil
}

// DecryptEncryptionKeyFile decrypts an Age-encrypted encryption key file
// using the provided Age identity file. It refuses to proceed if the
// identity file is world-readable.
func DecryptEncryptionKeyFile(encryptionKeyFile, identityFile string) (string, error) {
	if err := checkIdentityFilePermissions(identityFile); err != nil {
		return "", err
	}

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
