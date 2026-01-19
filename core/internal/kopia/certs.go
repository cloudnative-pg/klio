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
