package walserver

import (
	"crypto/rand"
	"fmt"
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

	_, err = writer.Write(buffer)
	require.Nil(t, err)

	err = writer.CloseMarkDone()
	require.Nil(t, err)

	reader, err := NewWALReader(conn, "cluster-example", "0000001000000000000001FF")
	fmt.Print(err)
	require.Nil(t, err)
	assert.NotNil(t, reader)

	bufferCompare := make([]byte, 16*1024*1024)
	_, err = reader.Read(bufferCompare)
	require.Nil(t, err)

	err = reader.Close()
	require.Nil(t, err)
}
