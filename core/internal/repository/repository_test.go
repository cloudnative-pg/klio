package repository

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	t.Run("should fail on non-existent repository", func(t *testing.T) {
		opts := Options{
			FS:       afero.NewMemMapFs(),
			Password: "test-password",
		}

		conn, err := Open(opts)
		assert.Nil(t, conn)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "while opening configuration file")
	})

	t.Run("should open initialized repository", func(t *testing.T) {
		opts := Options{
			FS:       afero.NewMemMapFs(),
			Password: "test-password",
		}

		err := Initialize(opts)
		require.NoError(t, err)

		conn, err := Open(opts)
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.NotNil(t, conn.config)
		assert.NotNil(t, conn.masterKey)
		assert.NotNil(t, conn.fs)

		conn.Close()
	})

	t.Run("should fail with wrong password", func(t *testing.T) {
		opts := Options{
			FS:       afero.NewMemMapFs(),
			Password: "correct-password",
		}

		err := Initialize(opts)
		require.NoError(t, err)

		// Try to open with wrong password
		wrongOpts := Options{
			FS:       opts.FS,
			Password: "wrong-password",
		}

		conn, err := Open(wrongOpts)
		assert.Nil(t, conn)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "while recovering the master key")
	})

	t.Run("should open with empty password", func(t *testing.T) {
		opts := Options{
			FS:       afero.NewMemMapFs(),
			Password: "", // Empty password
		}

		err := Initialize(opts)
		require.NoError(t, err)

		conn, err := Open(opts)
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.NotNil(t, conn.config)
		assert.NotNil(t, conn.masterKey)
		assert.NotNil(t, conn.fs)

		conn.Close()
	})

	t.Run("should fail if config file is corrupted", func(t *testing.T) {
		opts := Options{
			FS:       afero.NewMemMapFs(),
			Password: "test-password",
		}

		err := Initialize(opts)
		require.NoError(t, err)

		// Corrupt the config file by overwriting it with invalid data
		err = afero.WriteFile(opts.FS, repositoryConfigFileName, []byte("this is not valid data"), 0o644)
		require.NoError(t, err)

		conn, err := Open(opts)
		assert.Nil(t, conn)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "while decoding JSON from configuration file")
	})
}

func TestFileExists(t *testing.T) {
	fs := afero.NewMemMapFs()

	t.Run("should return false for non-existent file", func(t *testing.T) {
		exists, err := FileExists(fs, "non-existent-file")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("should return true for existing file", func(t *testing.T) {
		// Create a file
		file, err := fs.Create("test-file")
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)

		exists, err := FileExists(fs, "test-file")
		require.NoError(t, err)
		assert.True(t, exists)
	})
}
