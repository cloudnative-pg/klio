package repository

import (
	"fmt"
	"io"

	"github.com/minio/sio"
)

// ErrIVGeneration is raised when random IV bytes cannot be generated.
var ErrIVGeneration = fmt.Errorf("error while generating random IV bytes")

// ProtectWriter protects the writer by encrypting data with the master key.
func (c *Connection) ProtectWriter(out io.Writer) (io.WriteCloser, error) {
	result, err := sio.EncryptWriter(out, sio.Config{
		Key: c.masterKey,
	})
	if err != nil {
		return nil, fmt.Errorf("while creating protected writer: %w", err)
	}

	return result, nil
}

// ProtectReader protects the reader by decrypting data with the master key.
func (c *Connection) ProtectReader(in io.Reader) (io.Reader, error) {
	result, err := sio.DecryptReader(in, sio.Config{
		Key: c.masterKey,
	})
	if err != nil {
		return nil, fmt.Errorf("while creating protected reader: %w", err)
	}

	return result, nil
}
