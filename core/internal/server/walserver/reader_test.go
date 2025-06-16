package walserver

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
)

func TestWALReaderBlockSplit(t *testing.T) {
	opts := repository.Options{
		Path:     t.TempDir(),
		Password: "this-password",
	}

	err := repository.Initialize(opts)
	require.NoError(t, err)

	conn, err := repository.Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	defer conn.Close()

	readerNonExisting, err := NewReader(conn, "cluster-example", "0000001000000000000001FF")
	assert.Nil(t, readerNonExisting)
	require.ErrorIs(t, err, os.ErrNotExist)

	const fileLen = uint64(16 * 1024 * 1024)
	writer, err := NewWriter(conn, "cluster-example", "0000001000000000000001FF", fileLen)
	require.NoError(t, err)
	assert.NotNil(t, writer)

	buffer := make([]byte, fileLen)
	_, _ = rand.Read(buffer)

	err = writer.WriteBlock(buffer)
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	reader, err := NewReader(conn, "cluster-example", "0000001000000000000001FF")
	require.NoError(t, err)
	assert.NotNil(t, reader)
	assert.Equal(t, fileLen, reader.GetFileLength())

	// Read the splitted blocks
	var wBlocks bytes.Buffer
	for {
		innerBlock, err := reader.ReadBlock()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		_, _ = wBlocks.Write(innerBlock)
	}
	require.Equal(t, len(buffer), wBlocks.Len())
	require.Equal(t, buffer, wBlocks.Bytes())

	err = reader.Close()
	require.NoError(t, err)
}

func TestReaderWriterBlocks(t *testing.T) {
	// Step 1: write two blocks to the file
	opts := repository.Options{
		Path:     t.TempDir(),
		Password: "this-password",
	}

	err := repository.Initialize(opts)
	require.NoError(t, err)

	conn, err := repository.Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	defer conn.Close()

	const fileLen = uint64(145)
	writer, err := NewWriter(conn, "cluster-example", "0000001000000000000001F8", fileLen)
	require.NoError(t, err)
	require.NotNil(t, writer)

	block1 := []byte("this-test")
	err = writer.WriteBlock(block1)
	require.NoError(t, err)

	block2 := []byte("toast-is-good")
	err = writer.WriteBlock(block2)
	require.NoError(t, err)

	err = writer.Flush()
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	// Step 2: open the compressed file
	reader, err := NewReader(conn, "cluster-example", "0000001000000000000001F8")
	require.NoError(t, err)
	assert.NotNil(t, reader)
	assert.Equal(t, fileLen, reader.GetFileLength())

	// Step 2.1: read the first block
	block1Read, err := reader.ReadBlock()
	require.NoError(t, err)
	assert.Equal(t, block1, block1Read)

	// Step 2.2: read the second block
	block2Read, err := reader.ReadBlock()
	require.NoError(t, err)
	assert.Equal(t, block2, block2Read)

	err = reader.Close()
	require.NoError(t, err)
}
