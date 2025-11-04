package repository

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitialize(t *testing.T) {
	fs := afero.NewMemMapFs()

	err := Initialize(Options{
		FS:       fs,
		Password: "testpassword",
	})
	require.NoError(t, err)

	exists, err := FileExists(fs, repositoryConfigFileName)
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
	for b.Loop() {
		_, err := createNewRepositoryConfiguration("random-pwd")
		require.NoError(b, err)
	}
}

func BenchmarkCreateMasterKey(b *testing.B) {
	for b.Loop() {
		_, err := createNewMasterKey("random-pwd")
		require.NoError(b, err)
	}
}
