package klioserver

import (
	"os"
	"path"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/fileutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnterpriseDB/klio/internal/klioserver/repository"
)

func TestWriter(t *testing.T) {
	opts := repository.Options{
		Path:     t.TempDir(),
		Password: "this-password",
	}

	err := repository.Initialize(opts)
	require.NoError(t, err)

	conn, err := repository.Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	writer, err := NewWALWriter(conn, "cluster-example", "0000001000000000000001F")
	require.NoError(t, err)
	require.NotNil(t, writer)

	block := []byte("this-test")
	_, err = writer.Write(block)
	require.NoError(t, err)

	err = writer.Flush()
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	expectedPath := path.Join(opts.Path, "cluster-example", "0000001000000000", "0000001000000000000001F")
	exists, err := fileutils.FileExists(expectedPath)
	require.NoError(t, err)
	assert.True(t, exists)

	data, err := os.Stat(expectedPath)
	require.NoError(t, err)
	assert.Greater(t, data.Size(), int64(len(block)))
}
