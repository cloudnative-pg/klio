package repository

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.uber.org/multierr"
	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iterations         = 600_000
	masterKeyLen             = 32
	envelopeEncryptionKeyLen = 32
	keySaltLen               = 128
)

// ErrInvalidSaltFormat is raised when the encryption key salt is not valid.
var ErrInvalidSaltFormat = fmt.Errorf("invalid key salt format")

// ErrInvalidNonceFormat is raised when the encryption nonce is not valid.
var ErrInvalidNonceFormat = fmt.Errorf("invalid nonce format")

// ErrInvalidEncryptedKeyFormat is raised when the encrypted key is not valid.
var ErrInvalidEncryptedKeyFormat = fmt.Errorf("invalid encrypted key format")

// ErrInvalidPassword is raised when the password is not valid.
var ErrInvalidPassword = fmt.Errorf("invalid password")

// envelopedMasterKey represents a master key enveloped
// with a password.
type envelopedMasterKey struct {
	nonce      []byte
	cipherText []byte
	salt       []byte
	iterations int
}

func createNewMasterKey(password string) ([]byte, error) {
	masterKeySalt := make([]byte, keySaltLen)
	if _, err := rand.Read(masterKeySalt); err != nil {
		return nil, fmt.Errorf("while generating random master key salt: %w", err)
	}

	return pbkdf2.Key([]byte(password), masterKeySalt, pbkdf2Iterations, masterKeyLen, sha256.New), nil
}

func envelopeMasterKey(masterKey []byte, password string) (*envelopedMasterKey, error) {
	passwordSalt := make([]byte, keySaltLen)
	_, err := rand.Read(passwordSalt)
	if err != nil {
		return nil, fmt.Errorf("while generating random master key salt: %w", err)
	}

	encryptionKey := pbkdf2.Key([]byte(password), passwordSalt, pbkdf2Iterations, envelopeEncryptionKeyLen, sha256.New)
	encryptionBlock, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("while generating encryption block: %w", err)
	}

	// Important.
	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("while generating random nonce: %w", err)
	}

	aesgcm, err := cipher.NewGCM(encryptionBlock)
	if err != nil {
		panic(err.Error())
	}

	ciphertext := aesgcm.Seal(nil, nonce, masterKey, nil)

	return &envelopedMasterKey{
		nonce:      nonce,
		cipherText: ciphertext,
		salt:       passwordSalt,
		iterations: pbkdf2Iterations,
	}, nil
}

// RecoverMasterKey recovers the master key from the repository configuration.
func (c *KlioRepositoryConfig) RecoverMasterKey(password string) ([]byte, error) {
	var err error

	for i := range c.Keys {
		masterKey, keyErr := c.Keys[i].RecoverMasterKey(password)
		if keyErr == nil {
			return masterKey, nil
		}

		err = multierr.Append(err, keyErr)
	}

	return nil, err //nolint:wrapcheck
}

// RecoverMasterKey recovers the master key from the repository configuration.
func (c *KlioRepositoryKey) RecoverMasterKey(password string) ([]byte, error) {
	// Recover Salt
	passwordSalt, err := hex.DecodeString(c.Salt)
	if err != nil {
		return nil, ErrInvalidSaltFormat
	}

	// Recover nonce
	nonce, err := hex.DecodeString(c.Nonce)
	if err != nil {
		return nil, ErrInvalidNonceFormat
	}

	// Recover ciphered master key
	cipherText, err := hex.DecodeString(c.CipherText)
	if err != nil {
		return nil, ErrInvalidEncryptedKeyFormat
	}

	encryptionKey := pbkdf2.Key([]byte(password), passwordSalt, pbkdf2Iterations, masterKeyLen, sha256.New)
	encryptionBlock, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("while generating encryption block from recovered key: %w", err)
	}

	aesgcm, err := cipher.NewGCM(encryptionBlock)
	if err != nil {
		return nil, fmt.Errorf("while generating encryption block from recovered key: %w", err)
	}

	masterKey, err := aesgcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, ErrInvalidPassword
	}

	return masterKey, nil
}
