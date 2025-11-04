package walserver

import (
	"crypto/rand"
	"path"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/server/walserver/repository"
)

func TestWriter(t *testing.T) {
	opts := repository.Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := repository.Initialize(opts)
	require.NoError(t, err)

	conn, err := repository.Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	const fileLen = 123
	metrics := NewMetrics()
	writer, err := NewWriter(conn, "cluster-example", "0000001000000000000001F8", fileLen, metrics)
	require.NoError(t, err)
	require.NotNil(t, writer)

	block := []byte("this-test")
	err = writer.WriteBlock(t.Context(), block)
	require.NoError(t, err)

	err = writer.Flush()
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	expectedPath := path.Join("cluster-example", "0000001000000000", "0000001000000000000001F8")
	exists, err := repository.FileExists(opts.FS, expectedPath)
	require.NoError(t, err)
	assert.True(t, exists)

	data, err := opts.FS.Stat(expectedPath)
	require.NoError(t, err)
	assert.Greater(t, data.Size(), int64(len(block)))
}

func BenchmarkWriter(b *testing.B) {
	block := make([]byte, 100*1024)
	_, _ = rand.Read(block)

	opts := repository.Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := repository.Initialize(opts)
	require.NoError(b, err)

	conn, err := repository.Open(opts)
	assert.NotNil(b, conn)
	require.NoError(b, err)

	defer conn.Close()

	metrics := NewMetrics()
	writer, err := NewWriter(
		conn, "cluster-example", "0000001000000000000001FF", uint64(len(block)*b.N), metrics) //nolint:gosec
	require.NoError(b, err)
	assert.NotNil(b, writer)

	b.ResetTimer()
	b.SetBytes(int64(len(block) * b.N))
	for range b.N {
		err := writer.WriteBlock(b.Context(), block)
		require.NoError(b, err)

		err = writer.Flush()
		require.NoError(b, err)
	}

	err = writer.CloseMarkDone()
	require.NoError(b, err)
}
