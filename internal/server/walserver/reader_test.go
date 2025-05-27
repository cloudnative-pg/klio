package walserver

import (
	"crypto/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
)

func TestWALReader(t *testing.T) {
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

	readerNonExisting, err := NewWALReader(conn, "cluster-example", "0000001000000000000001FF")
	assert.Nil(t, readerNonExisting)
	require.ErrorIs(t, err, os.ErrNotExist)

	writer, err := NewWALWriter(conn, "cluster-example", "0000001000000000000001FF")
	require.Nil(t, err)
	assert.NotNil(t, writer)

	buffer := make([]byte, 16*1024*1024)
	_, _ = rand.Read(buffer)

	err = writer.WriteBlock(buffer)
	require.Nil(t, err)

	err = writer.CloseMarkDone()
	require.Nil(t, err)

	reader, err := NewWALReader(conn, "cluster-example", "0000001000000000000001FF")
	require.Nil(t, err)
	assert.NotNil(t, reader)

	bufferCompare, err := reader.ReadBlock()
	require.Nil(t, err)
	require.Equal(t, buffer, bufferCompare)

	err = reader.Close()
	require.Nil(t, err)
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

	writer, err := NewWALWriter(conn, "cluster-example", "0000001000000000000001F8")
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
	reader, err := NewWALReader(conn, "cluster-example", "0000001000000000000001F8")
	require.Nil(t, err)
	assert.NotNil(t, reader)

	// Step 2.1: read the first block
	block1Read, err := reader.ReadBlock()
	require.Nil(t, err)
	assert.Equal(t, block1, block1Read)

	// Step 2.2: read the second block
	block2Read, err := reader.ReadBlock()
	require.Nil(t, err)
	assert.Equal(t, block2, block2Read)

	err = reader.Close()
	require.Nil(t, err)
}
