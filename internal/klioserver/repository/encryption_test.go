package repository

import (
	"path"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/fileutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitialize(t *testing.T) {
	tempDir := t.TempDir()

	err := Initialize(Options{
		Path:     tempDir,
		Password: "testpassword",
	})
	require.NoError(t, err)

	exists, err := fileutils.FileExists(path.Join(tempDir, repositoryConfigFileName))
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestPasswordRecovery(t *testing.T) {
	cfg, err := createNewRepositoryConfiguration("testpassword")
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	masterKey, err := cfg.RecoverMasterKey("testpassword")
	require.NoError(t, err)
	assert.NotNil(t, masterKey)
	assert.Len(t, masterKey, masterKeyLen)

	wrongPwd, err := cfg.RecoverMasterKey("wrongpassword")
	require.ErrorIs(t, err, ErrInvalidPassword)
	assert.Nil(t, wrongPwd)
}

func BenchmarkCreateRepo(b *testing.B) {
	for range b.N {
		_, err := createNewRepositoryConfiguration("random-pwd")
		require.NoError(b, err)
	}
}

func BenchmarkCreateMasterKey(b *testing.B) {
	for range b.N {
		_, err := createNewMasterKey("random-pwd")
		require.NoError(b, err)
	}
}
