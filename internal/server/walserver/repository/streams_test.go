package repository

import (
	"crypto/rand"
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreams(t *testing.T) {
	opts := Options{
		Path:     t.TempDir(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(t, err)

	conn, err := Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	testFilePath := path.Join(opts.Path, "testfile")
	testData := "test-writer"

	// Write to an encrypted file
	fileWrite, err := os.OpenFile(testFilePath, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec
	require.NoError(t, err)

	encryptedWriter, err := conn.ProtectWriter(fileWrite)
	require.NoError(t, err)

	_, err = encryptedWriter.Write([]byte(testData))
	require.NoError(t, err)

	err = encryptedWriter.Close()
	require.NoError(t, err)

	// Look at the size
	data, err := os.Stat(testFilePath)
	require.NoError(t, err)

	assert.Greater(t, data.Size(), int64(10))

	// Read it back again
	fileRead, err := os.OpenFile(testFilePath, os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)

	encryptedReader, err := conn.ProtectReader(fileRead)
	require.NoError(t, err)

	bytes, err := io.ReadAll(encryptedReader)
	require.NoError(t, err)

	assert.Equal(t, testData, string(bytes))
}

func BenchmarkWriting(b *testing.B) {
	opts := Options{
		Path:     b.TempDir(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(b, err)

	conn, err := Open(opts)
	assert.NotNil(b, conn)
	require.NoError(b, err)

	testFilePath := path.Join(opts.Path, "testfile")

	b.ResetTimer()

	// Write to an encrypted file
	fileWrite, err := os.OpenFile(testFilePath, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec
	require.NoError(b, err)

	encryptedWriter, err := conn.ProtectWriter(fileWrite)
	require.NoError(b, err)

	blockSize := 512 * 1024
	block := make([]byte, blockSize)
	_, err = rand.Read(block)
	require.NoError(b, err)

	for range b.N {
		_, err = encryptedWriter.Write(block)
		require.NoError(b, err)

		err = fileWrite.Sync()
		require.NoError(b, err)
	}

	err = encryptedWriter.Close()
	require.NoError(b, err)

	data, err := os.Stat(testFilePath)
	require.NoError(b, err)

	bytes := int64(blockSize * b.N)
	assert.Greater(b, data.Size(), bytes, "requested size %v, actual size %v", bytes, data.Size())
	b.SetBytes(bytes)
}
