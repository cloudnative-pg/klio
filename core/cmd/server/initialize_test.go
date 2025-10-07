package server

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanInitRepoDirectory(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = afero.WriteFile(fs, "/nonEmptyDirectory/test.txt", []byte("test"), 0o600)
	_ = fs.Mkdir("/emptyDirectory", 0o777)

	v, err := canInitRepoDirectory(fs, "/nonEmptyDirectory")
	require.NoError(t, err)
	assert.False(t, v, "should not be able to init existing non-empty directory")

	v, err = canInitRepoDirectory(fs, "/emptyDirectory")
	require.NoError(t, err)
	assert.True(t, v, "should be able to init existing empty directory")

	v, err = canInitRepoDirectory(fs, "/nonExistingDirectory")
	require.NoError(t, err)
	assert.True(t, v, "should be able to init non existing directory")

	// error should be returned because the path is not a directory
	v, err = canInitRepoDirectory(fs, "/nonEmptyDirectory/test.txt")
	require.Error(t, err)
	assert.False(t, v, "should not be able to init when cannot read directory contents")
}
