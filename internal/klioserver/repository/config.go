package repository

import (
	"encoding/hex"
)

// KlioRepositoryConfig is the structure of a Klio repository
// configuration file.
type KlioRepositoryConfig struct {
	Kind       string              `json:"kind"`
	APIVersion string              `json:"apiVersion"`
	Keys       []KlioRepositoryKey `json:"encryptionKeys"`
}

// KlioRepositoryKey represent an encrypted Klio repository
// key.
type KlioRepositoryKey struct {
	Nonce      string `json:"nonce"`
	CipherText string `json:"cipherText"`
	Salt       string `json:"salt"`
	Iterations int    `json:"iterations"`
}

// createNewRepositoryConfiguration creates a new repository configuration
// with the passed password.
func createNewRepositoryConfiguration(password string) (*KlioRepositoryConfig, error) {
	// Important.
	// This is nothing fancy: it's just a standard envelope technique to decouple
	// the master key from the repository password as the latter can change or
	// multiple passwords can be added.

	// step 1. create master key
	masterKey, err := createNewMasterKey(password)
	if err != nil {
		return nil, err
	}

	// step 2. protect the master key with the password
	envelopedKey, err := envelopeMasterKey(masterKey, password)
	if err != nil {
		return nil, err
	}

	return &KlioRepositoryConfig{
		Kind:       "KlioRepositoryConfig",
		APIVersion: "v1",
		Keys: []KlioRepositoryKey{
			{
				Nonce:      hex.EncodeToString(envelopedKey.nonce),
				CipherText: hex.EncodeToString(envelopedKey.cipherText),
				Salt:       hex.EncodeToString(envelopedKey.salt),
				Iterations: envelopedKey.iterations,
			},
		},
	}, nil
}
