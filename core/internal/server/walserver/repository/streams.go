package repository

import (
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/s2"
	"github.com/minio/sio"
)

// WrapBlock protects a block encrypting and compressing it.
func (c *Connection) WrapBlock(block []byte) ([]byte, error) {
	var buffer bytes.Buffer

	encryptingWriter, err := c.protectWriter(&buffer)
	if err != nil {
		return nil, fmt.Errorf("while creating encrypted block: %w", err)
	}

	compressingWriter := s2.NewWriter(encryptingWriter, s2.WriterBlockSize(64*1024))

	if _, err := compressingWriter.Write(block); err != nil {
		return nil, fmt.Errorf("while compressing and encrypting WAL block: %w", err)
	}

	if err := compressingWriter.Close(); err != nil {
		return nil, fmt.Errorf("while closing compressed WAL block: %w", err)
	}

	if err := encryptingWriter.Close(); err != nil {
		return nil, fmt.Errorf("while closing encrypted WAL block: %w", err)
	}

	return buffer.Bytes(), nil
}

// UnwrapBlock reads back a block, decompressing and decrypting it.
func (c *Connection) UnwrapBlock(block []byte) ([]byte, error) {
	buffer := bytes.NewBuffer(block)

	decryptingReader, err := c.protectReader(buffer)
	if err != nil {
		return nil, fmt.Errorf("while opening decrypting WAL block: %w", err)
	}

	decompressingReader := s2.NewReader(decryptingReader)
	result, err := io.ReadAll(decompressingReader)
	if err != nil {
		return nil, fmt.Errorf("while reading compressed and encrypted WAL block: %w", err)
	}

	return result, nil
}

// protectWriter protects the writer by encrypting data with the master key.
func (c *Connection) protectWriter(out io.Writer) (io.WriteCloser, error) {
	result, err := sio.EncryptWriter(out, sio.Config{
		Key: c.masterKey,
	})
	if err != nil {
		return nil, fmt.Errorf("while creating protected writer: %w", err)
	}

	return result, nil
}

// protectReader protects the reader by decrypting data with the master key.
func (c *Connection) protectReader(in io.Reader) (io.Reader, error) {
	result, err := sio.DecryptReader(in, sio.Config{
		Key: c.masterKey,
	})
	if err != nil {
		return nil, fmt.Errorf("while creating protected reader: %w", err)
	}

	return result, nil
}
