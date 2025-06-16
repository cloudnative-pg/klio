package walserver

import (
	"os"
	"path"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/fileutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
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

	const fileLen = 123
	writer, err := NewWriter(conn, "cluster-example", "0000001000000000000001F8", fileLen)
	require.NoError(t, err)
	require.NotNil(t, writer)

	block := []byte("this-test")
	err = writer.WriteBlock(block)
	require.NoError(t, err)

	err = writer.Flush()
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	expectedPath := path.Join(opts.Path, "cluster-example", "0000001000000000", "0000001000000000000001F8")
	exists, err := fileutils.FileExists(expectedPath)
	require.NoError(t, err)
	assert.True(t, exists)

	data, err := os.Stat(expectedPath)
	require.NoError(t, err)
	assert.Greater(t, data.Size(), int64(len(block)))
}
